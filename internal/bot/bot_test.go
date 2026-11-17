package bot

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/ses"
)

type mockSES struct {
	sent bool
}

func (m *mockSES) SendEmailWithContext(ctx aws.Context, input *ses.SendEmailInput, opts ...request.Option) (*ses.SendEmailOutput, error) {
	m.sent = true
	return &ses.SendEmailOutput{}, nil
}

func TestNotifyBot_notify(t *testing.T) {
	conf := &Config{
		NotifyEmail: "to@example.com",
		FromEmail:   "from@example.com",
		AwsRegion:   "us-east-1",
	}
	nicknames := map[string]bool{"alice": false}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	bot, err := NewNotifyBot(conf, log, nicknames)
	if err != nil {
		t.Fatal(err)
	}
	mock := &mockSES{}
	bot.sesClient = mock

	bot.notify(context.Background(), "test message")
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called")
	}
}

func TestHandleISONResponse_online_offline(t *testing.T) {
	conf := &Config{}
	nicknames := map[string]bool{"alice": false, "bob": true, "marlene": false}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	bot, err := NewNotifyBot(conf, log, nicknames)
	if err != nil {
		t.Fatal(err)
	}
	mock := &mockSES{}
	bot.sesClient = mock

	// alice comes online
	parts := []string{"", "303", "notifybot", ":alice"}
	bot.handleISONResponse(context.Background(), parts)
	if !bot.nicknames["alice"] {
		t.Error("alice should be marked online")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for alice online")
	}
	mock.sent = false // reset

	// marlene comes online with trailing space
	parts = []string{"", "303", "notifybot", ":marlene "}
	bot.handleISONResponse(context.Background(), parts)
	if !bot.nicknames["marlene"] {
		t.Error("marlene should be marked online")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for marlene online")
	}
	mock.sent = false // reset

	// marlene goes offline (empty ISON response)
	parts = []string{"", "303", "notifybot", ":"}
	bot.handleISONResponse(context.Background(), parts)
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
	bot.handleISONResponse(context.Background(), parts)
	if bot.nicknames["bob"] {
		t.Error("bob should be marked offline")
	}
	if mock.sent {
		t.Error("SES SendEmail should NOT be called for bob offline again (no state change)")
	}
}

func TestHandleISONResponse_case_insensitive(t *testing.T) {
	conf := &Config{}
	nicknames := map[string]bool{"Alice": false, "BoB": false, "MARLENE": false}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	bot, err := NewNotifyBot(conf, log, nicknames)
	if err != nil {
		t.Fatal(err)
	}
	mock := &mockSES{}
	bot.sesClient = mock

	// alice comes online as "alice" (lowercase)
	parts := []string{"", "303", "notifybot", ":alice"}
	bot.handleISONResponse(context.Background(), parts)
	if !bot.nicknames["Alice"] {
		t.Error("Alice should be marked online (case-insensitive match)")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for Alice online (case-insensitive)")
	}
	mock.sent = false // reset

	// bob comes online as "BOB" (uppercase)
	parts = []string{"", "303", "notifybot", ":BOB"}
	bot.handleISONResponse(context.Background(), parts)
	if !bot.nicknames["BoB"] {
		t.Error("BoB should be marked online (case-insensitive match)")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for BoB online (case-insensitive)")
	}
	mock.sent = false // reset

	// marlene comes online as "mArLeNe" (mixed case)
	parts = []string{"", "303", "notifybot", ":mArLeNe"}
	bot.handleISONResponse(context.Background(), parts)
	if !bot.nicknames["MARLENE"] {
		t.Error("MARLENE should be marked online (case-insensitive match)")
	}
	if !mock.sent {
		t.Error("Expected SES SendEmail to be called for MARLENE online (case-insensitive)")
	}
}
