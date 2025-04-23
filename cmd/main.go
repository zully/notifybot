package main

import (
	"log/slog"
	"notifybot/internal/bot"
	"os"
	"strings"
)

func loadConfigFromEnv() *bot.Config {
	requiredVars := []string{"SERVER", "PORT", "BOT_NAME", "NOTIFY_EMAIL", "FROM_EMAIL", "AWS_REGION"}
	for _, v := range requiredVars {
		if os.Getenv(v) == "" {
			slog.Debug("Environment variable is required but not set", "error", v)
		}
	}

	return &bot.Config{
		Server:      os.Getenv("SERVER"),
		Port:        os.Getenv("PORT"),
		BotName:     os.Getenv("BOT_NAME"),
		Channels:    strings.Split(os.Getenv("CHANNELS"), ","),
		NotifyEmail: os.Getenv("NOTIFY_EMAIL"),
		FromEmail:   os.Getenv("FROM_EMAIL"),
		SleepMin:    os.Getenv("SLEEP_MIN"),
		AwsRegion:   os.Getenv("AWS_REGION"),
	}
}

func loadNicknamesFromEnv() map[string]bool {
	nicknames := make(map[string]bool)
	for _, nickname := range strings.Split(os.Getenv("NICKNAMES"), ",") {
		nicknames[nickname] = false
	}
	return nicknames
}

func main() {
	// Create a logger using log/slog
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	log.Info("Starting NotifyBot")

	config := loadConfigFromEnv()
	log.Info("Configuration loaded successfully")
	nicknames := loadNicknamesFromEnv()

	notifyBot := bot.NewNotifyBot(config, log, nicknames)
	notifyBot.Run()
}
