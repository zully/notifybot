package bot

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
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

type sesSender interface {
	SendEmailWithContext(ctx aws.Context, input *ses.SendEmailInput, opts ...request.Option) (*ses.SendEmailOutput, error)
}

type NotifyBot struct {
	conf              *Config
	nicknames         map[string]bool
	log               *slog.Logger
	sleepDuration     time.Duration
	sesClient         sesSender
	metrics           *metrics
	retryDelay        time.Duration
	readTimeout       time.Duration
	heartbeatInterval time.Duration
}

func parseSleepDuration(config *Config, log *slog.Logger) time.Duration {
	if config.SleepMin != "" {
		duration, err := time.ParseDuration(config.SleepMin)
		if err != nil || duration <= 0 {
			log.Warn("Invalid polling interval; using 5 minutes", "duration", config.SleepMin)
			return 5 * time.Minute
		}
		return duration
	}
	log.Debug("Polling interval defaults to 5 minutes")
	return 5 * time.Minute
}

func NewNotifyBot(config *Config, log *slog.Logger, nicknames map[string]bool) (*NotifyBot, error) {
	log.Info("NotifyBot starting", "version", notifyBotVersion)
	var nicks []string
	for k := range nicknames {
		nicks = append(nicks, k)
	}
	slices.Sort(nicks)
	log.Info("Watching IRC nicknames", "count", len(nicks))
	log.Debug("Configured nicknames", "nicknames", nicks)

	sess, err := session.NewSession(&aws.Config{
		Region:     aws.String(config.AwsRegion),
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("create AWS session: %w", err)
	}

	return &NotifyBot{
		conf:              config,
		nicknames:         nicknames,
		log:               log,
		sleepDuration:     parseSleepDuration(config, log),
		sesClient:         ses.New(sess), // SES client for sending emails
		metrics:           newMetrics(len(nicknames)),
		retryDelay:        3 * time.Minute,
		readTimeout:       3 * time.Minute,
		heartbeatInterval: time.Minute,
	}, nil
}

func (b *NotifyBot) handleISONResponse(ctx context.Context, parts []string) {
	if len(parts) < 4 {
		b.log.Warn("Ignoring malformed ISON response")
		return
	}
	isonField := strings.TrimSpace(strings.TrimPrefix(strings.Join(parts[3:], " "), ":"))
	b.metrics.isonResponses.Inc()
	b.metrics.lastISON.SetToCurrentTime()
	var currentnicknames []string
	if isonField != "" {
		for _, n := range strings.Fields(isonField) {
			currentnicknames = append(currentnicknames, strings.ToLower(n))
		}
	}

	for nickname := range b.nicknames {
		lowerNick := strings.ToLower(nickname)
		isOnline := slices.Contains(currentnicknames, lowerNick)
		if isOnline && !b.nicknames[nickname] {
			b.log.Info("Friend is online", "nickname", nickname)
			b.nicknames[nickname] = true
			b.metrics.presenceChanges.WithLabelValues("online").Inc()
			b.notify(ctx, fmt.Sprintf("%s is online", nickname))
		} else if !isOnline && b.nicknames[nickname] {
			b.log.Info("Friend is offline", "nickname", nickname)
			b.nicknames[nickname] = false
			b.metrics.presenceChanges.WithLabelValues("offline").Inc()
			b.notify(ctx, fmt.Sprintf("%s is offline", nickname))
		}
	}
	online := 0
	for _, present := range b.nicknames {
		if present {
			online++
		}
	}
	b.metrics.online.Set(float64(online))
}

func (b *NotifyBot) notify(ctx context.Context, msg string) {
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
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err := b.sesClient.SendEmailWithContext(ctx, input)
	b.metrics.emailDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		b.metrics.emails.WithLabelValues("error").Inc()
		b.log.Error("Error sending email", "error", err)
		return
	}

	b.metrics.emails.WithLabelValues("success").Inc()
	b.log.Debug("Notification email sent")
}

// Run starts the bot until the process exits.
func (b *NotifyBot) Run() { b.RunContext(context.Background()) }

// RunContext stops connection attempts and polling when ctx is canceled.
func (b *NotifyBot) RunContext(ctx context.Context) {
	for ctx.Err() == nil {
		b.metrics.connectionAttempts.Inc()
		b.log.Info("Connecting to IRC server", "server", b.conf.Server, "port", b.conf.Port)
		conn, err := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(b.conf.Server, b.conf.Port))
		if err == nil {
			err = b.runConnection(ctx, conn)
		}
		if ctx.Err() != nil {
			break
		}
		b.metrics.connectionErrors.Inc()
		b.log.Warn("IRC connection ended; retry scheduled", "retry_in", b.retryDelay, "error", err)
		timer := time.NewTimer(b.retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	b.log.Info("NotifyBot stopped")
}

func (b *NotifyBot) runConnection(ctx context.Context, conn net.Conn) error {
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClose()
	defer b.metrics.connected.Set(0)
	defer b.metrics.online.Set(0)
	write := func(format string, args ...any) error {
		if err := conn.SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil {
			return err
		}
		_, err := fmt.Fprintf(conn, format, args...)
		return err
	}
	currentNick := b.conf.BotName
	collisions := 0
	if err := write("NICK %s\r\nUSER notifybot 8 * :NotifyBot\r\n", currentNick); err != nil {
		return err
	}
	type incoming struct {
		line string
		err  error
	}
	messages := make(chan incoming)
	done := make(chan struct{})
	defer close(done)
	go func() {
		scanner := bufio.NewScanner(conn)
		for {
			if err := conn.SetReadDeadline(time.Now().Add(b.readTimeout)); err != nil {
				break
			}
			if !scanner.Scan() {
				break
			}
			select {
			case messages <- incoming{line: scanner.Text()}:
			case <-done:
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = fmt.Errorf("server closed the connection")
		}
		select {
		case messages <- incoming{err: err}:
		case <-done:
		}
	}()
	var ticker *time.Ticker
	var ticks <-chan time.Time
	defer func() {
		if ticker != nil {
			ticker.Stop()
		}
	}()
	poll := func() error {
		keys := make([]string, 0, len(b.nicknames))
		for nick := range b.nicknames {
			keys = append(keys, nick)
		}
		slices.Sort(keys)
		if err := write("ISON %s\r\n", strings.Join(keys, " ")); err != nil {
			return err
		}
		b.metrics.isonRequests.Inc()
		return nil
	}
	heartbeat := time.NewTicker(b.heartbeatInterval)
	defer heartbeat.Stop()
	registered := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if err := write("PING :notifybot\r\n"); err != nil {
				return err
			}
		case <-ticks:
			if err := poll(); err != nil {
				return err
			}
		case message := <-messages:
			if message.err != nil {
				return message.err
			}
			parts := strings.SplitN(message.line, " ", 4)
			if len(parts) == 0 || parts[0] == "" {
				continue
			}
			command := parts[0]
			paramsStart := 1
			if strings.HasPrefix(command, ":") {
				if len(parts) < 2 {
					continue
				}
				command = parts[1]
				paramsStart = 2
			}
			b.log.Debug("IRC command received", "command", command)
			switch command {
			case "PING":
				if len(parts) <= paramsStart {
					continue
				}
				if err := write("PONG %s\r\n", strings.Join(parts[paramsStart:], " ")); err != nil {
					return err
				}
			case "001":
				if registered {
					continue
				}
				registered = true
				b.metrics.connected.Set(1)
				b.log.Info("Registered with IRC server", "server", b.conf.Server)
				for _, channel := range b.conf.Channels {
					if channel == "" {
						continue
					}
					if err := write("JOIN %s\r\n", channel); err != nil {
						return err
					}
				}
				if err := poll(); err != nil {
					return err
				}
				ticker = time.NewTicker(b.sleepDuration)
				ticks = ticker.C
			case "303":
				b.handleISONResponse(ctx, parts)
			case "433":
				collisions++
				if collisions > 9 {
					return fmt.Errorf("nickname collision limit reached")
				}
				base := b.conf.BotName
				if len(base) > 8 {
					base = base[:8]
				}
				currentNick = fmt.Sprintf("%s%d", base, collisions)
				b.log.Warn("Nickname in use; trying another", "nickname", currentNick)
				if err := write("NICK %s\r\n", currentNick); err != nil {
					return err
				}
			case "PRIVMSG":
				if len(parts) == 4 && parts[3] == ":\x01VERSION\x01" {
					nick := strings.Split(strings.TrimPrefix(parts[0], ":"), "!")[0]
					if err := write("NOTICE %s :\x01VERSION NotifyBot %s\x01\r\n", nick, notifyBotVersion); err != nil {
						return err
					}
				}
			case "ERROR":
				return fmt.Errorf("IRC server reported an error")
			}
		}
	}
}
