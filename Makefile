APP := arbbot

.PHONY: build clean up down

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/$(APP) ./cmd/bot

clean:
	rm -rf bin/

up:
	docker compose up -d --build

down:
	docker compose down
