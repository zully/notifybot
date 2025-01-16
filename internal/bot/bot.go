package bot

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type NotifyBot struct {
	server           string
	port             string
	nickname         string
	channels         []string
	nicknamesToCheck []string
	onlineNicknames  map[string]bool
	log              *logrus.Logger
}

func NewNotifyBot(server, port, nickname string, channels []string, nicknamesToCheck []string, log *logrus.Logger) *NotifyBot {
	return &NotifyBot{
		server:           server,
		port:             port,
		nickname:         nickname,
		channels:         channels,
		nicknamesToCheck: nicknamesToCheck,
		onlineNicknames:  make(map[string]bool),
		log:              log,
	}
}

func (b *NotifyBot) Run() {
	b.log.Infof("Attempting to connect to server: %s", b.server)
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", b.server, b.port))
	if err != nil {
		b.log.Errorf("Error connecting to server: %s", err)
		return
	}
	defer conn.Close()

	fmt.Fprintf(conn, "NICK %s\r\n", b.nickname)
	fmt.Fprintf(conn, "USER %s 8 * :%s\r\n", b.nickname, b.nickname)

	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			msg := scanner.Text()
			b.log.Info(msg)
			parts := strings.Split(msg, " ")
			if len(parts) > 0 {
				if parts[0] == "PING" {
					fmt.Fprintf(conn, "PONG %s\r\n", parts[1])
					b.log.Infof("PONG %s", parts[1])
				}
				if parts[1] == "303" {
					b.handleISONResponse(parts)
				}
				if parts[1] == "VERSION" {
					nickname := strings.TrimPrefix(parts[0], ":")
					fmt.Fprintf(conn, "NOTICE %s :%s\r\n", nickname, "NotifyBot version 0.1a")
					b.log.Infof("NOTICE %s :%s", nickname, "NotifyBot version 0.1a")
				}
				if parts[1] == "QUIT" || parts[1] == "PART" {
					b.handleQuitOrPart(msg)
				}
				if strings.Contains(msg, fmt.Sprintf("NOTICE %s :on", b.nickname)) {
					b.log.Infof("Connected to server: %s", b.server)

					// join any channels specified in the config
					for _, channel := range b.channels {
						fmt.Fprintf(conn, "JOIN %s\r\n", channel)
					}

					// Check who is online every 5 minutes
					go func() {
						for {
							b.checkNicknames(conn)
							time.Sleep(5 * time.Minute)
						}
					}()
				}
			}
		}
	}()

	// Temporary block to allow for manual input
	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		fmt.Fprintf(conn, "%s\r\n", input.Text())
	}
}

func (b *NotifyBot) checkNicknames(conn net.Conn) {
	nicknames := strings.Join(b.nicknamesToCheck, " ")
	fmt.Fprintf(conn, "ISON %s\r\n", nicknames)
}

func (b *NotifyBot) handleISONResponse(parts []string) {
	if len(parts) > 3 {
		// WORKING ON THE NOTIFICATION LOGIC
		currentOnlineNicknames := strings.Fields(parts[3])
		for _, nickname := range b.nicknamesToCheck {
			if contains(currentOnlineNicknames, nickname) {
				if !b.onlineNicknames[nickname] {
					b.log.Infof("The following friend is now online: %s\n", nickname)
					b.onlineNicknames[nickname] = true
					// b.notify(fmt.Sprintf("%s is now online", nickname))
				}
			} else {
				if b.onlineNicknames[nickname] {
					b.log.Infof("The following friend has gone offline: %s\n", nickname)
					b.onlineNicknames[nickname] = false
					// b.notify(fmt.Sprintf("%s has gone offline", nickname))
				}
			}
		}
	}
}

func (b *NotifyBot) handleQuitOrPart(msg string) {
	parts := strings.Split(msg, " ")
	if len(parts) > 2 {
		nickname := strings.TrimPrefix(parts[0], ":")
		if b.onlineNicknames[nickname] {
			b.log.Infof("The following friend has left IRC: %s\n", nickname)
			b.onlineNicknames[nickname] = false
			// b.notify(fmt.Sprintf("%s has left IRC", nickname))
		}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (b *NotifyBot) notify(message string) {
	// Placeholder for notification logic (e.g., send SMS or email)
	b.log.Infof("Notification: %s\n", message)
}
