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
	conf            *Config
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

func (b *NotifyBot) setNickname(conn net.Conn) {
	// Send the NICK and USER commands to identify the bot
	fmt.Fprintf(conn, "NICK %s\r\n", b.conf.BotName)
	fmt.Fprintf(conn, "USER %s 8 * :%s\r\n", b.conf.BotName, b.conf.BotName)
}

func (b *NotifyBot) Connect() (net.Conn, error) {
	b.log.Infof("Attempting to connect to server: %s", b.conf.Server)
	conn, err := net.Dial("tcp", net.JoinHostPort(b.conf.Server, b.conf.Port))
	if err != nil {
		b.log.Errorf("Error connecting to server: %s", err)
		return nil, err
	}
	defer conn.Close()
	return conn, nil
}

func (b *NotifyBot) Run() {
	conn, _ := b.Connect()
	b.setNickname(conn)

	// read incoming messages from the server and act on them
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		b.log.Info(msg)
		slices := strings.Split(msg, " ")

		if len(slices) > 0 {
			// TODO: functionality for handling other server messages
			//  ERROR :Your host is trying to (re)connect too fast -- throttled
			//  ERROR :Closing Link: notifybot by Chicago.IL.US.Undernet.Org (Ping timeout)
			// TODO: Sleep 5 minutes and attempt to reconnect if disconnected
			switch slices[1] {
			// :Chicago.IL.US.Undernet.Org 303 notifybot :
			case "303": // ISON response
				if len(slices) > 3 {
					b.handleISONResponse(slices)
				}
			// :Chicago.IL.US.Undernet.Org 433 * notifybot :Nickname is already in use.
			case "433": // Nickname already in use
				b.log.Errorf("Nickname %s is already in use. Appending _ to the end of the nick.", b.conf.BotName)
				b.conf.BotName = fmt.Sprintf("%s_", b.conf.BotName)
				b.setNickname(conn)
			// FIX VERSION response
			// :Nickname!~ident@10.10.10.10 PRIVMSG notifybot :\x01VERSION \x01"
			case "PRIVMSG": // Respond to certain private messages
				b.log.Info(slices[3])
				if slices[2] == b.conf.BotName && slices[3] == ":\x01VERSION \x01" {
					nickname := strings.TrimPrefix(slices[0], ":")
					nickname = strings.Split(nickname, "!")[0] // Remove the host part
					fmt.Fprintf(conn, "NOTICE %s :NotifyBot %s\r\n", nickname, notifyBotVersion)
					b.log.Infof("NOTICE %s :NotifyBot %s", nickname, notifyBotVersion)
				}
			}

			// PING :Chicago.IL.US.Undernet.Org"
			if slices[0] == "PING" {
				fmt.Fprintf(conn, "PONG %s\r\n", slices[1])
				b.log.Infof("PONG %s", slices[1])
				// :Chicago.IL.US.Undernet.Org NOTICE notifybot :on 1 ca 1(4) ft 10(10) tr
			} else if strings.Contains(msg, fmt.Sprintf("NOTICE %s :on", b.conf.BotName)) {
				b.log.Infof("Connected to server: %s", b.conf.Server)

				// join any channels specified in the config
				if len(b.conf.Channels) != 0 {
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
