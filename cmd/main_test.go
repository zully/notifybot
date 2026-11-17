package main

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func validEnv(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{"SERVER": "127.0.0.1", "PORT": "6667", "BOT_NAME": "notifybot", "NOTIFY_EMAIL": "to@example.com", "FROM_EMAIL": "from@example.com", "AWS_REGION": "us-east-1", "NICKNAMES": "alice,bob", "CHANNELS": "", "METRICS_ADDR": ""} {
		t.Setenv(name, value)
	}
}

func TestConfigValidation(t *testing.T) {
	for _, name := range []string{"SERVER", "PORT", "BOT_NAME", "NOTIFY_EMAIL", "FROM_EMAIL", "AWS_REGION", "NICKNAMES"} {
		t.Run(name, func(t *testing.T) {
			validEnv(t)
			t.Setenv(name, "")
			if _, err := loadConfigFromEnv(); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("got %v, want missing %s", err, name)
			}
		})
	}
	for _, value := range []string{"0", "65536", "abc", "-1"} {
		t.Run("port"+value, func(t *testing.T) {
			validEnv(t)
			t.Setenv("PORT", value)
			if _, err := loadConfigFromEnv(); err == nil {
				t.Fatal("accepted invalid port")
			}
		})
	}
	for _, value := range []string{", ,", "alice\r\nQUIT", "alice bob", "alice:foo"} {
		t.Run("nicknames", func(t *testing.T) {
			validEnv(t)
			t.Setenv("NICKNAMES", value)
			if _, err := loadConfigFromEnv(); err == nil {
				t.Fatal("accepted invalid nicknames")
			}
		})
	}
}

func TestEnvironmentLists(t *testing.T) {
	validEnv(t)
	t.Setenv("NICKNAMES", " alice, bob, ,alice,")
	t.Setenv("CHANNELS", " #one, ,#two,")
	config, err := loadConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Channels, []string{"#one", "#two"}) {
		t.Fatal(config.Channels)
	}
	if !reflect.DeepEqual(loadNicknamesFromEnv(), map[string]bool{"alice": false, "bob": false}) {
		t.Fatal(loadNicknamesFromEnv())
	}
}

func TestMetricsListenerFailure(t *testing.T) {
	validEnv(t)
	t.Setenv("METRICS_ADDR", "invalid-address")
	if err := run(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("expected metrics listener error")
	}
}

func TestMetricsServerShutdown(t *testing.T) {
	validEnv(t)
	t.Setenv("METRICS_ADDR", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
}
