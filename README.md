# notifybot

A simple IRC bot that will notify you via SMS/E-mail when your friends are online.

## Project Structure

```
cmd
└── main.go
internal
├── bot
│   └── bot.go
go.mod
LICENSE
README.md
```

## Installation

To install the project, clone the repository and navigate to the project directory:

```
git clone <repository-url>
cd <repository>
```

Then, run the following command to download the dependencies:

```
go mod tidy
```

## Usage

To run the bot, execute the following command:

```
go run cmd/main.go
```

Make sure to configure your IRC server settings and notification preferences in the code before running the bot.

## Contributing

Feel free to submit issues or pull requests for improvements or bug fixes.