package main

import (
	"notifybot/internal/bot"
)

func main() {
	notifyBot := bot.NewNotifyBot("chicago.il.us.undernet.org", "6667", "notifybot", []string{"gopherbot"})
	notifyBot.Run()
}
