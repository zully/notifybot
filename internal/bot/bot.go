package bot

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	server   = "irc.freenode.net:6667"
	channel  = "#yourchannel"
	nickname = "notifybot"
)

var nicknamesToCheck = []string{"friend1", "friend2", "friend3"}
var onlineNicknames = make(map[string]bool)

func Run() {
	conn, err := net.Dial("tcp", server)
	if err != nil {
		fmt.Println("Error connecting to server:", err)
		return
	}
	defer conn.Close()

	fmt.Fprintf(conn, "NICK %s\r\n", nickname)
	fmt.Fprintf(conn, "USER %s 8 * :%s\r\n", nickname, nickname)
	fmt.Fprintf(conn, "JOIN %s\r\n", channel)

	go func() {
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			msg := scanner.Text()
			fmt.Println(msg)
			if strings.Contains(msg, "PING") {
				fmt.Fprintf(conn, "PONG %s\r\n", strings.Split(msg, " ")[1])
			}
			if strings.Contains(msg, "ISON") {
				handleISONResponse(msg)
			}
			if strings.Contains(msg, "QUIT") || strings.Contains(msg, "PART") {
				handleQuitOrPart(msg)
			}
		}
	}()

	go func() {
		for {
			checkNicknames(conn)
			time.Sleep(5 * time.Minute)
		}
	}()

	input := bufio.NewScanner(os.Stdin)
	for input.Scan() {
		fmt.Fprintf(conn, "PRIVMSG %s :%s\r\n", channel, input.Text())
	}
}

func checkNicknames(conn net.Conn) {
	nicknames := strings.Join(nicknamesToCheck, " ")
	fmt.Fprintf(conn, "ISON %s\r\n", nicknames)
}

func handleISONResponse(msg string) {
	parts := strings.Split(msg, ":")
	if len(parts) > 1 {
		currentOnlineNicknames := strings.Fields(parts[1])
		for _, nickname := range nicknamesToCheck {
			if contains(currentOnlineNicknames, nickname) {
				if !onlineNicknames[nickname] {
					fmt.Printf("The following friend is now online: %s\n", nickname)
					onlineNicknames[nickname] = true
					// notify.Notify(fmt.Sprintf("%s is now online", nickname))
				}
			} else {
				if onlineNicknames[nickname] {
					fmt.Printf("The following friend has gone offline: %s\n", nickname)
					onlineNicknames[nickname] = false
					// notify.Notify(fmt.Sprintf("%s has gone offline", nickname))
				}
			}
		}
	}
}

func handleQuitOrPart(msg string) {
	parts := strings.Split(msg, " ")
	if len(parts) > 2 {
		nickname := strings.TrimPrefix(parts[0], ":")
		if onlineNicknames[nickname] {
			fmt.Printf("The following friend has left IRC: %s\n", nickname)
			onlineNicknames[nickname] = false
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

func Notify(message string) {
	// Placeholder for notification logic (e.g., send SMS or email)
	fmt.Printf("Notification: %s\n", message)
}
