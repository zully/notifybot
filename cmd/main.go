package main

import (
	"log"
	"notifybot/internal/bot"
	"os"
	"strings"
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

func loadNicknamesFromEnv() map[string]bool {
	nicknames := make(map[string]bool)
	for _, nickname := range strings.Split(os.Getenv("NICKNAMES"), ",") {
		nicknames[nickname] = false
	}
	return nicknames
}

func main() {
	log := log.New(os.Stdout, "notifybot: ", log.LstdFlags)
	config := loadConfigFromEnv()
	nicknames := loadNicknamesFromEnv()

	notifyBot := bot.NewNotifyBot(config, log, nicknames)
	notifyBot.Run()
}
