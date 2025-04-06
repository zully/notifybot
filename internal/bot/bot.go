package bot

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

const notifyBotVersion = "v0.1h"

type Config struct {
	Server      string
	Port        string
	BotName     string
	Channels    []string
	Nicknames   []string
	NotifyEmail string
	FromEmail   string
	SleepMin    string
	AwsRegion   string
}

type NotifyBot struct {
	conf            *Config // Config holds the configuration for the bot
	onlineNicknames map[string]bool
	log             *logrus.Logger
	sleepDuration   time.Duration
	sesClient       *ses.SES
}

func NewNotifyBot(config *Config, log *logrus.Logger) *NotifyBot {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(config.AwsRegion),
	})
	if err != nil {
		log.Fatalf("Failed to create AWS session: %s", err)
	}

	return &NotifyBot{
		conf:            config,
		onlineNicknames: make(map[string]bool),
		log:             log,
		sleepDuration: func() time.Duration {
			if config.SleepMin != "" {
				duration, err := time.ParseDuration(config.SleepMin)
				if err != nil {
					log.Errorf("Error parsing duration %s, using default of 5 minutes: %s", config.SleepMin, err)
					return 5 * time.Minute // Default to 5 minutes if parsing fails
				}
				return duration
			}
			log.Errorf("'SleepMin' not provided in config, defaulting to 5 minutes")
			return 5 * time.Minute // Default to 5 minutes if not provided
		}(),
		sesClient: ses.New(sess), // SES client for sending emails
	}
}

func (b *NotifyBot) Run() {
	b.log.Infof("Attempting to connect to server: %s", b.conf.Server)
	conn, err := net.Dial("tcp", net.JoinHostPort(b.conf.Server, b.conf.Port))
	if err != nil {
		b.log.Errorf("Error connecting to server: %s", err)
		return
	}
	defer conn.Close()

	// Send the NICK and USER commands to identify the bot
	fmt.Fprintf(conn, "NICK %s\r\n", b.conf.BotName)
	fmt.Fprintf(conn, "USER %s 8 * :%s\r\n", b.conf.BotName, b.conf.BotName)

	// read incoming messages from the server and act on them
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		b.log.Info(msg)
		slices := strings.Split(msg, " ")

		if len(slices) > 0 {
			// TODO: functionality for handling other server messages
			// ERROR :Your host is trying to (re)connect too fast -- throttled
			// INFO[0000] :Chicago.IL.US.Undernet.Org 433 * notifybot :Nickname is already in use.
			if slices[0] == "PING" {
				fmt.Fprintf(conn, "PONG %s\r\n", slices[1])
				b.log.Infof("PONG %s", slices[1])
			} else if slices[1] == "303" && len(slices) > 3 {
				b.handleISONResponse(slices)
			} else if slices[1] == "VERSION" { // not correctly responding to version requests
				nickname := strings.TrimPrefix(slices[0], ":")
				fmt.Fprintf(conn, "NOTICE %s :NotifyBot %s\r\n", nickname, notifyBotVersion)
				b.log.Infof("NOTICE %s :NotifyBot %s", nickname, notifyBotVersion)
			} else if strings.Contains(msg, fmt.Sprintf("NOTICE %s :on", b.conf.BotName)) {
				b.log.Infof("Connected to server: %s", b.conf.Server)

				// join any channels specified in the config
				if len(b.conf.Channels) == 0 {
					for _, channel := range b.conf.Channels {
						fmt.Fprintf(conn, "JOIN %s\r\n", channel)
					}
				}

				// Check who is online every X configured minutes
				go func() {
					for {
						nicknames := strings.Join(b.conf.Nicknames, " ")
						fmt.Fprintf(conn, "ISON %s\r\n", nicknames)
						time.Sleep(b.sleepDuration)
					}
				}()
			}
		}
	}
}

func (b *NotifyBot) handleISONResponse(parts []string) {
	parts[3] = strings.TrimPrefix(parts[3], ":") // Remove the leading colon from the response
	currentOnlineNicknames := parts[3:]          // The actual nicknames that are online, after the "ISON" command

	for _, nickname := range b.conf.Nicknames {
		if slices.Contains(currentOnlineNicknames, nickname) {
			if _, exists := b.onlineNicknames[nickname]; !exists || !b.onlineNicknames[nickname] {
				b.log.Infof("The following friend is now online: %s\n", nickname)
				b.onlineNicknames[nickname] = true
				b.notify(fmt.Sprintf("%s is online", nickname)) // Uncomment to enable notifications
			}
		} else {
			if _, exists := b.onlineNicknames[nickname]; exists && b.onlineNicknames[nickname] {
				b.log.Infof("The following friend is now offline: %s\n", nickname)
				b.onlineNicknames[nickname] = false
				b.notify(fmt.Sprintf("%s is offline", nickname)) // Uncomment to enable notifications
			}
		}
	}
}

func (b *NotifyBot) notify(msg string) {
	subject := "IRC Notification Event"

	// Load the Central Time location
	location, err := time.LoadLocation("America/Chicago")
	if err != nil {
		b.log.Errorf("Error loading location for Central Time: %s", err)
		location = time.UTC // Fallback to UTC if loading fails
	}

	// Get the current timestamp in Central Time
	timestamp := time.Now().In(location).Format("2006-01-02 15:04:05")

	// Append the timestamp to the message
	msg = fmt.Sprintf("[%s] %s", timestamp, msg)

	// Construct email input
	input := &ses.SendEmailInput{
		Destination: &ses.Destination{
			ToAddresses: []*string{
				aws.String(b.conf.NotifyEmail),
			},
		},
		Message: &ses.Message{
			Body: &ses.Body{
				Text: &ses.Content{
					Charset: aws.String("UTF-8"),
					Data:    aws.String(msg),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String("UTF-8"),
				Data:    aws.String(subject),
			},
		},
		Source: aws.String(b.conf.FromEmail),
	}

	// Send the email
	_, err = b.sesClient.SendEmail(input)
	if err != nil {
		b.log.Errorf("Error sending email: %s", err)
		return
	}

	b.log.Infof("Email sent successfully to %s", b.conf.NotifyEmail)
}
