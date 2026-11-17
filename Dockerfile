FROM golang:1.26-alpine3.23 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /app/notifybot ./cmd

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/notifybot ./notifybot
USER 65532:65532
ENTRYPOINT ["./notifybot"]
