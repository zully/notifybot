package bot

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ses"
)

type recordingSES struct {
	messages chan *ses.SendEmailInput
	err      error
}

func (m *recordingSES) SendEmailWithContext(ctx aws.Context, input *ses.SendEmailInput, opts ...request.Option) (*ses.SendEmailOutput, error) {
	m.messages <- input
	return &ses.SendEmailOutput{}, m.err
}

func testBot(t *testing.T, nicks map[string]bool) (*NotifyBot, *recordingSES) {
	t.Helper()
	b, err := NewNotifyBot(&Config{BotName: "notifybot", AwsRegion: "us-east-1", NotifyEmail: "to@example.com", FromEmail: "from@example.com", SleepMin: "1h"}, slog.New(slog.NewTextHandler(io.Discard, nil)), nicks)
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingSES{messages: make(chan *ses.SendEmailInput, 100)}
	b.sesClient = sender
	return b, sender
}

func TestSleepDuration(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{{"", 5 * time.Minute}, {"garbage", 5 * time.Minute}, {"0s", 5 * time.Minute}, {"-1m", 5 * time.Minute}, {"30s", 30 * time.Second}} {
		t.Run(tc.value, func(t *testing.T) {
			got := parseSleepDuration(&Config{SleepMin: tc.value}, slog.Default())
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestMultipleISONAndMetrics(t *testing.T) {
	b, sender := testBot(t, map[string]bool{"Alice": false, "bob": false})
	b.handleISONResponse(context.Background(), strings.Split(":server 303 notifybot :alice BOB", " "))
	if !b.nicknames["Alice"] || !b.nicknames["bob"] || len(sender.messages) != 2 {
		t.Fatal("multi-nickname response parsed incorrectly")
	}
	b.handleISONResponse(context.Background(), []string{":server", "303", "notifybot", ":alice BOB"})
	if len(sender.messages) != 2 {
		t.Fatal("duplicate notifications")
	}
	b.handleISONResponse(context.Background(), []string{":server", "303"})
	assertMetric(t, b, "notifybot_ison_responses_total 2")
	assertMetric(t, b, "notifybot_online_nicknames 2")
	assertMetric(t, b, `notifybot_email_attempts_total{result="success"} 2`)
	b.handleISONResponse(context.Background(), []string{":server", "303", "notifybot", ":"})
	assertMetric(t, b, "notifybot_online_nicknames 0")
	assertMetric(t, b, `notifybot_presence_changes_total{state="offline"} 2`)
}

func assertMetric(t *testing.T, b *NotifyBot, want string) {
	t.Helper()
	response := httptest.NewRecorder()
	b.MetricsHandler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), want+"\n") {
		t.Fatalf("missing %q in metrics: %s", want, response.Body.String())
	}
}

func TestEmailPayloadAndFailureMetrics(t *testing.T) {
	b, sender := testBot(t, nil)
	sender.err = errors.New("SES unavailable")
	b.notify(context.Background(), "alice is online")
	input := <-sender.messages
	if aws.StringValue(input.Source) != "from@example.com" || aws.StringValue(input.Destination.ToAddresses[0]) != "to@example.com" || aws.StringValue(input.Message.Subject.Data) != "IRC Notification Event" {
		t.Fatal("incorrect email routing or subject")
	}
	if !strings.HasSuffix(aws.StringValue(input.Message.Body.Text.Data), "UTC] alice is online") {
		t.Fatal("missing message or timestamp")
	}
	assertMetric(t, b, `notifybot_email_attempts_total{result="error"} 1`)
	assertMetric(t, b, "notifybot_email_duration_seconds_count 1")
}

func readLine(t *testing.T, c net.Conn, r *bufio.Reader) string {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(line)
}

func TestConnectionProtocolAndCancellation(t *testing.T) {
	b, _ := testBot(t, map[string]bool{"bob": false, "alice": false})
	var logs bytes.Buffer
	b.log = slog.New(slog.NewJSONHandler(&logs, nil))
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.runConnection(ctx, client) }()
	r := bufio.NewReader(server)
	for _, want := range []string{"NICK notifybot", "USER notifybot 8 * :NotifyBot"} {
		if got := readLine(t, server, r); got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	for _, msg := range []string{"", ":server", "PING", ":server PRIVMSG", ":server 303", "ERRORLESS"} {
		fmt.Fprintf(server, "%s\r\n", msg)
	}
	fmt.Fprint(server, "PING :token\r\n")
	if got := readLine(t, server, r); got != "PONG :token" {
		t.Fatal(got)
	}
	fmt.Fprint(server, ":server 433 * notifybot :In use\r\n")
	if got := readLine(t, server, r); got != "NICK notifybo1" {
		t.Fatal(got)
	}
	fmt.Fprint(server, ":server 001 notifybo1 :Welcome\r\n")
	if got := readLine(t, server, r); got != "ISON alice bob" {
		t.Fatal(got)
	}
	fmt.Fprint(server, ":server 001 notifybo1 :Welcome again\r\n:server NOTICE notifybo1 :on this server\r\nPING :after\r\n")
	if got := readLine(t, server, r); got != "PONG :after" {
		t.Fatalf("duplicate registration started extra polling: %s", got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("connection failed to stop")
	}
	assertMetric(t, b, "notifybot_irc_connected 0")
	if strings.Contains(logs.String(), "token") || strings.Contains(logs.String(), "IRC command received") {
		t.Fatal("protocol traffic leaked into INFO logs")
	}
}

func TestRetryAfterEOFAndCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	b, _ := testBot(t, nil)
	b.conf.Server, b.conf.Port, _ = net.SplitHostPort(listener.Addr().String())
	b.retryDelay = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { b.RunContext(ctx); close(done) }()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	readLine(t, conn, r)
	readLine(t, conn, r)
	start := time.Now()
	conn.Close()
	listener.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second))
	conn, err = listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < b.retryDelay {
		t.Fatal("retry happened without backoff")
	}
	r = bufio.NewReader(conn)
	readLine(t, conn, r)
	readLine(t, conn, r)
	conn.Close()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backoff failed to cancel")
	}
	assertMetric(t, b, "notifybot_connection_attempts_total 2")
}

func TestSilentConnectionTimesOut(t *testing.T) {
	b, _ := testBot(t, nil)
	b.readTimeout = 40 * time.Millisecond
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.runConnection(ctx, client) }()
	r := bufio.NewReader(server)
	readLine(t, server, r)
	readLine(t, server, r)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("silent connection did not return an error")
		}
	case <-time.After(time.Second):
		t.Fatal("silent connection did not time out")
	}
}

func TestClientHeartbeat(t *testing.T) {
	b, _ := testBot(t, nil)
	b.heartbeatInterval = 10 * time.Millisecond
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.runConnection(ctx, client) }()
	r := bufio.NewReader(server)
	readLine(t, server, r)
	readLine(t, server, r)
	if got := readLine(t, server, r); got != "PING :notifybot" {
		t.Fatal(got)
	}
	fmt.Fprint(server, ":server PING :prefixed\r\n")
	if got := readLine(t, server, r); got != "PONG :prefixed" {
		t.Fatal(got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not stop")
	}
}

type canceledSES struct{}

func (*canceledSES) SendEmailWithContext(ctx aws.Context, input *ses.SendEmailInput, opts ...request.Option) (*ses.SendEmailOutput, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestEmailCancellation(t *testing.T) {
	b, _ := testBot(t, nil)
	b.sesClient = &canceledSES{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { b.notify(ctx, "test"); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SES send did not honor cancellation")
	}
	assertMetric(t, b, `notifybot_email_attempts_total{result="error"} 1`)
}

func TestServerErrorEndsConnection(t *testing.T) {
	b, _ := testBot(t, nil)
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.runConnection(ctx, client) }()
	r := bufio.NewReader(server)
	readLine(t, server, r)
	readLine(t, server, r)
	fmt.Fprint(server, "ERROR :Closing link\r\n")
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected server error")
		}
	case <-time.After(time.Second):
		t.Fatal("server error did not end connection")
	}
}
