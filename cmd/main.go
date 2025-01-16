package main

import (
	"notifybot/internal/bot"
	"os"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Server    string   `yaml:"server"`
	Port      string   `yaml:"port"`
	BotName   string   `yaml:"botname"`
	Channels  []string `yaml:"channels"`
	Nicknames []string `yaml:"nicknames"`
}

func loadConfig(configPath string) (*Config, error) {
	config := &Config{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func main() {
	log := logrus.New()
	config, err := loadConfig("../config.yaml")
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	notifyBot := bot.NewNotifyBot(config.Server, config.Port, config.BotName, config.Channels, config.Nicknames, log)
	notifyBot.Run()
}
