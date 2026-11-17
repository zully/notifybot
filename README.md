# notifybot

An IRC presence bot that emails online/offline changes through Amazon SES.

## Run

Build with Go 1.26.11 or later (`go run ./cmd`), or use Docker:

```sh
docker build -t notifybot:local .
docker run --rm --env-file .env -p 127.0.0.1:9090:9090 notifybot:local
```

Create an untracked `.env` with these settings:

```dotenv
SERVER=us.undernet.org
PORT=6667
BOT_NAME=notifybot
NOTIFY_EMAIL=to@example.com
FROM_EMAIL=from@example.com
AWS_REGION=us-west-2
NICKNAMES=alice,bob
CHANNELS=#example
SLEEP_MIN=5m
LOG_LEVEL=INFO
METRICS_ADDR=:9090
```

`SERVER`, `PORT`, `BOT_NAME`, both email addresses, `AWS_REGION`, and at least one nickname are required. Lists are comma-separated; whitespace and empty entries are removed. `CHANNELS` is optional. `SLEEP_MIN` accepts a positive Go duration; missing, invalid, zero, or negative values use five minutes. Credentials use the AWS SDK credential chain; prefer an ECS task role or EC2 instance role. SES needs `ses:SendEmail`, a verified sending identity, and sandbox-compatible recipients when applicable. Never commit credentials or source private local scripts during testing.

## Connections and logging

The bot uses a fixed valid IRC username and tries bounded nickname alternatives on collisions. It registers with NICK/USER, waits for the `001` welcome, joins configured channels, then polls ISON immediately and at the configured interval. Only one polling timer runs per connection. Presence is retained across reconnects to avoid duplicate notifications for unchanged users; changes during an outage are detected on the first response after reconnecting.

Every failed connection or disconnect waits three minutes before retrying. Dial/write operations have 15-second timeouts. A client PING runs every minute; three minutes without a complete incoming line closes the connection and schedules a retry. SES operations have a 15-second total deadline. SIGINT/SIGTERM cancel socket operations, SES requests, and retry waits. A disconnect does not generate offline emails by itself.

Logs are structured JSON. `LOG_LEVEL` defaults to `INFO`; `DEBUG`, `WARN`, and `ERROR` are supported. INFO records registration and presence changes; WARN records retries and invalid polling intervals; ERROR records startup and email failures. Protocol commands are DEBUG events without raw message content.

## Metrics

Set `METRICS_ADDR` to enable `/metrics` (disabled when unset). For a host-only process use `127.0.0.1:9090`; in Docker use `:9090` and publish only to the intended interface. The endpoint has no authentication. Metrics use the `notifybot_` prefix:

| Metric suffix | Meaning |
| --- | --- |
| `irc_connected` | 1 after registration; 0 while disconnected |
| `connection_attempts_total`, `connection_errors_total` | TCP attempts and failed attempts/disconnects |
| `ison_requests_total`, `ison_responses_total` | Written polls and valid responses |
| `last_ison_response_timestamp_seconds` | Last valid response time; 0 before the first |
| `watched_nicknames`, `online_nicknames` | Configured count and observed online count; online is 0 while disconnected |
| `presence_changes_total{state}` | Online/offline transitions |
| `email_attempts_total{result}` | SES send operations by success/error |
| `email_duration_seconds` | Histogram of SES operation duration |

Labels contain only fixed state/result values. Check `irc_connected` together with the last-response timestamp to identify stale presence data. An SES success means SES accepted the email, rather than confirmed recipient delivery.

## Tests

```sh
go test -race ./...
go vet ./...
```

The optional integration test uses Debian's Undernet `ircu` package in a local Docker container and a mock SES client. It tests presence changes, duplicate suppression, and recovery after restarting the server. No real emails are sent.

```sh
docker compose -f compose.test.yaml up -d --build
IRC_TEST_ADDR=127.0.0.1:16667 IRC_TEST_RESTART=1 go test -race -v -run TestIRCUIntegration ./internal/bot
docker compose -f compose.test.yaml down
```

The test listener binds only `127.0.0.1:16667`. Run this against the dedicated test server; the restart option restarts that container. Keep generated coverage, logs, binaries, and other testing output outside Git.

## Known limitations

IRC uses plaintext TCP. Presence is sampled and held only in memory. SES delivery is synchronous; failed sends are logged/counted without an application-level delivery queue. Large nickname lists are not split into multiple IRC commands. Nicknames use ordinary case-insensitive comparison rather than negotiated IRC casemapping. The AWS SDK v1 dependency needs migration to v2.
