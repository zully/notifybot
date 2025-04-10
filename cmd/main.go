package main

import (
	"notifybot/internal/bot"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

func loadConfigFromEnv() *bot.Config {
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

func main() {
	log := logrus.New()
	config := loadConfigFromEnv()

	var onlineNicknames = make(map[string]bool)
	for _, nickname := range strings.Split(os.Getenv("NICKNAMES"), ",") {
		onlineNicknames[nickname] = false
	}

	notifyBot := bot.NewNotifyBot(config, log, onlineNicknames)
	notifyBot.Run()
}
