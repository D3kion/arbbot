FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bot ./cmd/bot

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -g '' appuser
COPY --from=build /bot /usr/local/bin/bot
USER appuser
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD pidof bot || exit 1
ENTRYPOINT ["bot"]
