FROM golang:1.24-alpine3.20 AS builder
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download
COPY . /app/
RUN go build -o /app/bin/notifybot /app/cmd/main.go

FROM alpine:3.20
WORKDIR /root/
COPY --from=builder /app/bin/notifybot .
CMD ["./notifybot"]