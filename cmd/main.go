package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"notifybot/internal/bot"
)

func splitList(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func loadConfigFromEnv() (*bot.Config, error) {
	required := []string{"SERVER", "PORT", "BOT_NAME", "NOTIFY_EMAIL", "FROM_EMAIL", "AWS_REGION", "NICKNAMES"}
	for _, name := range required {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			return nil, fmt.Errorf("required environment variable %s is missing", name)
		}
	}
	if strings.ContainsAny(os.Getenv("BOT_NAME"), " \t,:") {
		return nil, fmt.Errorf("BOT_NAME contains an invalid IRC token")
	}
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("PORT must be between 1 and 65535")
	}
	for _, name := range []string{"SERVER", "BOT_NAME", "CHANNELS", "NICKNAMES"} {
		if strings.ContainsAny(os.Getenv(name), "\r\n\x00") {
			return nil, fmt.Errorf("%s contains invalid control characters", name)
		}
	}
	for _, name := range []string{"BOT_NAME", "CHANNELS", "NICKNAMES"} {
		for _, item := range splitList(os.Getenv(name)) {
			if strings.ContainsAny(item, " \t:") {
				return nil, fmt.Errorf("%s contains an invalid IRC token", name)
			}
		}
	}
	if len(loadNicknamesFromEnv()) == 0 {
		return nil, fmt.Errorf("NICKNAMES must contain at least one nickname")
	}
	return &bot.Config{
		Server: os.Getenv("SERVER"), Port: os.Getenv("PORT"), BotName: os.Getenv("BOT_NAME"),
		Channels: splitList(os.Getenv("CHANNELS")), NotifyEmail: os.Getenv("NOTIFY_EMAIL"),
		FromEmail: os.Getenv("FROM_EMAIL"), SleepMin: os.Getenv("SLEEP_MIN"), AwsRegion: os.Getenv("AWS_REGION"),
	}, nil
}

func loadNicknamesFromEnv() map[string]bool {
	nicknames := make(map[string]bool)
	for _, nickname := range splitList(os.Getenv("NICKNAMES")) {
		nicknames[nickname] = false
	}
	return nicknames
}

func run(ctx context.Context, log *slog.Logger) error {
	config, err := loadConfigFromEnv()
	if err != nil {
		return err
	}
	notifyBot, err := bot.NewNotifyBot(config, log, loadNicknamesFromEnv())
	if err != nil {
		return err
	}
	serverErrors := make(chan error, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if addr := os.Getenv("METRICS_ADDR"); addr != "" {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listen for metrics: %w", err)
		}
		mux := http.NewServeMux()
		mux.Handle("/metrics", notifyBot.MetricsHandler())
		server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
		defer server.Close()
		go func() {
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErrors <- fmt.Errorf("metrics server: %w", err)
				cancel()
			}
		}()
		log.Info("Metrics endpoint listening", "address", listener.Addr().String())
	}
	notifyBot.RunContext(ctx)
	select {
	case err := <-serverErrors:
		return err
	default:
		return nil
	}
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var level slog.Level
	if value := os.Getenv("LOG_LEVEL"); value != "" {
		if err := level.UnmarshalText([]byte(value)); err != nil {
			log.Error("Invalid LOG_LEVEL", "error", err)
			os.Exit(1)
		}
	}
	log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, log); err != nil {
		log.Error("NotifyBot failed", "error", err)
		os.Exit(1)
	}
}
