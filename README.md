# notifybot

A simple IRC bot that will notify you via Amazon SES when your friends are online.

## Project Structure

```
cmd
└── main.go
internal
├── bot
│   └── bot.go
go.mod
go.sum
LICENSE
README.md
```

## Installation

This bot is intended to run in a docker container in amazon ECS or on EC2

## Usage

To run the bot in ECS you can built the docker image and publish to ECR

```
docker build -t notifybot:latest .
```

The bot gets its configuration from ENV variables you can configure in your task-definition.json

```
SERVER="irc.freenode.org"
PORT="6667"
BOT_NAME="notifybot"
NOTIFY_EMAIL="notify@email.com"
FROM_EMAIL="from@email.com"
NICKNAMES="john,bob,rob"
CHANNELS="#chan123,#chan234"
SLEEP_MIN="5m"
AWS_REGION="us-west-2"
```

## Contributing

Feel free to submit issues or pull requests for improvements or bug fixes.