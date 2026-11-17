package bot

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func ircuClient(t *testing.T, addr, nick string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	fmt.Fprintf(conn, "NICK %s\r\nUSER tester 0 * :Integration test\r\n", nick)
	scanner := bufio.NewScanner(conn)
	registered := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PING ") {
			fmt.Fprintf(conn, "PONG %s\r\n", strings.TrimPrefix(line, "PING "))
		}
		if strings.Contains(line, " 001 ") {
			registered = true
			break
		}
		if strings.Contains(line, " 433 ") {
			t.Fatalf("test nickname already in use: %s", nick)
		}
	}
	if !registered {
		t.Fatalf("ircu registration failed: %v", scanner.Err())
	}
	conn.SetDeadline(time.Time{})
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PING ") {
				fmt.Fprintf(conn, "PONG %s\r\n", strings.TrimPrefix(line, "PING "))
			}
		}
	}()
	return conn
}

func awaitEmail(t *testing.T, sender *recordingSES, want string) {
	t.Helper()
	select {
	case input := <-sender.messages:
		if !strings.HasSuffix(*input.Message.Body.Text.Data, want) {
			t.Fatalf("got %q, want suffix %q", *input.Message.Body.Text.Data, want)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("timed out waiting for %s", want)
	}
}

func TestIRCUIntegration(t *testing.T) {
	addr := os.Getenv("IRC_TEST_ADDR")
	if addr == "" {
		t.Skip("set IRC_TEST_ADDR after starting compose.test.yaml")
	}
	ircuClient(t, addr, "notifybot")
	alice := ircuClient(t, addr, "nbalice")
	b, sender := testBot(t, map[string]bool{"nbalice": false, "nbbob": false})
	b.conf.Server, b.conf.Port, _ = net.SplitHostPort(addr)
	b.sleepDuration = 200 * time.Millisecond
	b.retryDelay = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { b.RunContext(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("bot did not stop")
		}
	})
	awaitEmail(t, sender, "nbalice is online")
	ircuClient(t, addr, "nbbob")
	awaitEmail(t, sender, "nbbob is online")
	select {
	case message := <-sender.messages:
		t.Fatalf("duplicate notification: %s", *message.Message.Body.Text.Data)
	case <-time.After(500 * time.Millisecond):
	}
	alice.Close()
	awaitEmail(t, sender, "nbalice is offline")
	if os.Getenv("IRC_TEST_RESTART") == "1" {
		command := exec.CommandContext(ctx, "docker", "compose", "-f", "../../compose.test.yaml", "restart", "ircu")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("restart ircu: %v: %s", err, output)
		}
		awaitEmail(t, sender, "nbbob is offline")
		ircuClient(t, addr, "nbalice")
		awaitEmail(t, sender, "nbalice is online")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("bot did not shut down")
	}
	assertMetric(t, b, "notifybot_irc_connected 0")
}
