package bot

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"golang.org/x/exp/slices"
)

const notifyBotVersion = "v0.3c"

type Config struct {
	Server      string
	Port        string
	BotName     string
	Channels    []string
	NotifyEmail string
	FromEmail   string
	SleepMin    string
	AwsRegion   string
}

// Mock SES client implementing only the SendEmail method needed for testing
// Use an interface for sesClient to allow mocking

type sesSender interface {
	SendEmail(input *ses.SendEmailInput) (*ses.SendEmailOutput, error)
}

type NotifyBot struct {
	conf          *Config
	nicknames     map[string]bool
	log           *slog.Logger
	sleepDuration time.Duration
	sesClient     sesSender
	connected     bool
}

func NewNotifyBot(config *Config, log *slog.Logger, nicknames map[string]bool) *NotifyBot {
	log.Info("NotifyBot starting", "version", notifyBotVersion)
	var nicks []string
	for k := range nicknames {
		nicks = append(nicks, k)
	}
	log.Info("Notifying on nicknames", "nicknames", nicks)

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(config.AwsRegion),
	})
	if err != nil {
		log.Error("Failed to create AWS session", "error", err)
	}

	return &NotifyBot{
		conf:      config,
		nicknames: nicknames,
		log:       log,
		sleepDuration: func() time.Duration {
			if config.SleepMin != "" {
				duration, err := time.ParseDuration(config.SleepMin)
				if err != nil {
					log.Error("Error parsing duration", "duration", config.SleepMin, "error", err)
					return 5 * time.Minute // Default to 5 minutes if parsing fails
				}
				return duration
			}
			log.Error("'SleepMin' not provided in config, defaulting to 5 minutes")
			return 5 * time.Minute // Default to 5 minutes if not provided
		}(),
		sesClient: ses.New(sess), // SES client for sending emails
		connected: false,
	}
}

func (b *NotifyBot) setNickname(conn net.Conn) {
	// Send the NICK and USER commands to identify the bot
	fmt.Fprintf(conn, "NICK %s\r\n", b.conf.BotName)
	fmt.Fprintf(conn, "USER %s 8 * :%s\r\n", b.conf.BotName, b.conf.BotName)
}

func (b *NotifyBot) connect() (net.Conn, error) {
	b.log.Info("Attempting to connect to server", "server", b.conf.Server)
	conn, err := net.Dial("tcp", net.JoinHostPort(b.conf.Server, b.conf.Port))
	if err != nil {
		b.log.Error("Error connecting to server", "error", err)
		return nil, err
	}
	return conn, nil
}

func (b *NotifyBot) reconnect() net.Conn {
	for {
		b.log.Info("Attempting to reconnect to server", "server", b.conf.Server)
		conn, err := b.connect()
		if err == nil {
			b.log.Info("Reconnected to server successfully", "server", b.conf.Server)
			return conn
		}
		b.log.Error("Reconnection attempt failed", "error", err)
		time.Sleep(3 * time.Minute) // Wait 3 minutes before retrying
	}
}

func (b *NotifyBot) handleISONResponse(parts []string) {
	// Remove the leading colon and split nicknames, trim whitespace
	isonField := strings.TrimPrefix(parts[3], ":")
	isonField = strings.TrimSpace(isonField)
	var currentnicknames []string
	if isonField != "" {
		for _, n := range strings.Fields(isonField) {
			currentnicknames = append(currentnicknames, strings.TrimSpace(n))
		}
	}

	for nickname := range b.nicknames {
		isOnline := slices.Contains(currentnicknames, nickname)
		if isOnline && !b.nicknames[nickname] {
			b.log.Info("The following friend is now online:", "nickname", strings.TrimSuffix(nickname, "\n"))
			b.nicknames[nickname] = true
			b.notify(fmt.Sprintf("%s is online", nickname))
		} else if !isOnline && b.nicknames[nickname] {
			b.log.Info("The following friend is now offline:", "nickname", strings.TrimSuffix(nickname, "\n"))
			b.nicknames[nickname] = false
			b.notify(fmt.Sprintf("%s is offline", nickname))
		}
	}
}

func (b *NotifyBot) notify(msg string) {
	subject := "IRC Notification Event"

	// Use UTC for timestamp
	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")

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
	_, err := b.sesClient.SendEmail(input)
	if err != nil {
		b.log.Error("Error sending email", "error", err)
		return
	}

	b.log.Info("Email sent successfully", "recipient", b.conf.NotifyEmail)
}

func (b *NotifyBot) Run() {
reconnectLoop:
	for {
		conn, err := b.connect()
		if err != nil {
			b.log.Error("Failed to connect to server", "error", err)
			conn = b.reconnect() // Attempt to reconnect if the initial connection fails
		}
		b.setNickname(conn)

		// read incoming messages from the server and act on them
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			msg := scanner.Text()
			b.log.Info(msg)
			parts := strings.Split(msg, " ")

			if len(parts) > 0 {
				switch {
				case parts[0] == "PING":
					fmt.Fprintf(conn, "PONG %s\r\n", parts[1])
					b.log.Info("PONG", "id", parts[1])
				case parts[1] == "303": // ISON response
					if len(parts) > 3 {
						b.handleISONResponse(parts)
					}
				case parts[1] == "433": // Nickname in use
					b.log.Error("Nickname is already in use. Appending _ to the end of the nick.", "nickname", b.conf.BotName)
					b.conf.BotName = fmt.Sprintf("%s_", b.conf.BotName)
					b.setNickname(conn)
				case parts[1] == "PRIVMSG":
					if strings.Contains((parts[3]), "VERSION") {
						nickname := strings.TrimPrefix(parts[0], ":")
						nickname = strings.Split(nickname, "!")[0]
						fmt.Fprintf(conn, "NOTICE %s :NotifyBot %s\r\n", nickname, notifyBotVersion)
						b.log.Info("Version request acknowledged", "nickname", nickname, "version", notifyBotVersion)
					}
				case parts[0] == "ERROR":
					b.log.Error("Server error, attempting to reconnect", "error", msg)
					conn.Close()
					continue reconnectLoop // Go back to the top and reconnect
				case strings.Contains(msg, fmt.Sprintf("NOTICE %s :on", b.conf.BotName)):
					b.log.Info("Connected to server", "server", b.conf.Server)
					b.connected = true

					// join any channels specified in the config
					if b.conf.Channels[0] != "" {
						for _, channel := range b.conf.Channels {
							fmt.Fprintf(conn, "JOIN %s\r\n", channel)
						}
					}

					// Check who is online every X configured minutes
					var keys []string
					for k := range b.nicknames {
						keys = append(keys, k)
					}
					nicknames := strings.Join(keys, " ")

					go func(c net.Conn, nicks string, sleep time.Duration) {
						for {
							_, err := fmt.Fprintf(c, "ISON %s\r\n", nicks)
							if err != nil {
								return // exit goroutine if write fails
							}
							time.Sleep(sleep)
						}
					}(conn, nicknames, b.sleepDuration)
				}
			}
		}

		// If we reach here, scanner.Scan() returned false (connection lost or error)
		if err := scanner.Err(); err != nil {
			b.log.Error("Error reading from server, attempting to reconnect", "error", err)
		} else {
			b.log.Error("Disconnected from server, attempting to reconnect")
		}
		conn.Close()
		// Loop will restart and reconnect
	}
}
