package bot

import (
	"bytes"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/ses"
)

// Mock SES client implementing only the SendEmail method needed for testing
type mockSES struct {
	ses.SES
	sent bool
}

func (m *mockSES) SendEmail(input *ses.SendEmailInput) (*ses.SendEmailOutput, error) {
	m.sent = true
	return &ses.SendEmailOutput{}, nil
}

// Dummy net.Conn for testing
type dummyConn struct {
	bytes.Buffer
}

func (d *dummyConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (d *dummyConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (d *dummyConn) Close() error                       { return nil }
func (d *dummyConn) LocalAddr() net.Addr                { return nil }
func (d *dummyConn) RemoteAddr() net.Addr               { return nil }
func (d *dummyConn) SetDeadline(t time.Time) error      { return nil }
func (d *dummyConn) SetReadDeadline(t time.Time) error  { return nil }
func (d *dummyConn) SetWriteDeadline(t time.Time) error { return nil }

func TestNotifyBot_notify(t *testing.T) {
	conf := &Config{
		NotifyEmail: "to@example.com",
		FromEmail:   "from@example.com",
		AwsRegion:   "us-east-1",
	}
	nicknames := map[string]bool{"alice": false}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	bot := NewNotifyBot(conf, log, nicknames)
	mock := &mockSES{}
	bot.sesClient = mock // now valid, as sesClient is an interface

	bot.notify("test message")
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called")
	}
}

func TestHandleISONResponse_online_offline(t *testing.T) {
	conf := &Config{}
	nicknames := map[string]bool{"alice": false, "bob": true, "marlene": false}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	bot := NewNotifyBot(conf, log, nicknames)
	mock := &mockSES{}
	bot.sesClient = mock

	// alice comes online
	parts := []string{"", "303", "notifybot", ":alice"}
	bot.handleISONResponse(parts)
	if !bot.nicknames["alice"] {
		t.Error("alice should be marked online")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for alice online")
	}
	mock.sent = false // reset

	// marlene comes online with trailing space
	parts = []string{"", "303", "notifybot", ":marlene "}
	bot.handleISONResponse(parts)
	if !bot.nicknames["marlene"] {
		t.Error("marlene should be marked online")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for marlene online")
	}
	mock.sent = false // reset

	// marlene goes offline (empty ISON response)
	parts = []string{"", "303", "notifybot", ":"}
	bot.handleISONResponse(parts)
	if bot.nicknames["marlene"] {
		t.Error("marlene should be marked offline")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for marlene offline")
	}
	mock.sent = false // reset

	// bob goes offline (was true at start)
	// Only notify if state changes from true to false
	// Since bob was set offline above, no notification should be sent again
	parts = []string{"", "303", "notifybot", ":"}
	bot.handleISONResponse(parts)
	if bot.nicknames["bob"] {
		t.Error("bob should be marked offline")
	}
	if mock.sent {
		t.Error("SES SendEmail should NOT be called for bob offline again (no state change)")
	}
}

func TestSetNickname(t *testing.T) {
	conf := &Config{BotName: "testbot"}
	nicknames := map[string]bool{}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	bot := NewNotifyBot(conf, log, nicknames)
	conn := &dummyConn{}
	bot.setNickname(conn)
	// No panic means success
}
