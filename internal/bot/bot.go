package bot

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

type NotifyBot struct {
	server           string
	port             string
	nickname         string
	nicknamesToCheck []string
	onlineNicknames  map[string]bool
	log              *logrus.Logger
}

func NewNotifyBot(server, port, nickname string, nicknamesToCheck []string) *NotifyBot {
	return &NotifyBot{
		server:           server,
		port:             port,
		nickname:         nickname,
		nicknamesToCheck: nicknamesToCheck,
		onlineNicknames:  make(map[string]bool),
		log:              logrus.New(),
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
			if strings.Contains(msg, "PING") {
				response := fmt.Sprintf("PONG %s\r\n", strings.Split(msg, " ")[1])
				fmt.Fprint(conn, response)
				b.log.Info(response)
			}
			if strings.Contains(msg, "ISON") {
				b.handleISONResponse(msg)
			}
			if strings.Contains(msg, "VERSION") {
				parts := strings.Split(msg, " ")
				if len(parts) > 0 && parts[1] == "VERSION" {
					nickname := strings.TrimPrefix(parts[0], ":")
					response := fmt.Sprintf("NOTICE %s :%s\r\n", nickname, "NotifyBot version 0.1a")
					fmt.Fprint(conn, response)
					b.log.Info(response)
				}
			}
			if strings.Contains(msg, "QUIT") || strings.Contains(msg, "PART") {
				b.handleQuitOrPart(msg)
			}
		}
	}()

	// go func() {
	// 	for {
	// 		b.checkNicknames(conn)
	// 		time.Sleep(5 * time.Minute)
	// 	}
	// }()

	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		fmt.Fprintf(conn, "%s\r\n", input.Text())
	}
}

func (b *NotifyBot) checkNicknames(conn net.Conn) {
	nicknames := strings.Join(b.nicknamesToCheck, " ")
	fmt.Fprintf(conn, "ISON %s\r\n", nicknames)
}

func (b *NotifyBot) handleISONResponse(msg string) {
	parts := strings.Split(msg, ":")
	if len(parts) > 1 {
		currentOnlineNicknames := strings.Fields(parts[1])
		for _, nickname := range b.nicknamesToCheck {
			if contains(currentOnlineNicknames, nickname) {
				if !b.onlineNicknames[nickname] {
					b.log.Infof("The following friend is now online: %s\n", nickname)
					b.onlineNicknames[nickname] = true
					// notify.Notify(fmt.Sprintf("%s is now online", nickname))
				}
			} else {
				if b.onlineNicknames[nickname] {
					b.log.Infof("The following friend has gone offline: %s\n", nickname)
					b.onlineNicknames[nickname] = false
					// notify.Notify(fmt.Sprintf("%s has gone offline", nickname))
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
			// notify.Notify(fmt.Sprintf("%s has left IRC", nickname))
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
