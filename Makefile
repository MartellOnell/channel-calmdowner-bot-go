.PHONY: build run clean docker-build tidy test

build:
	go build -o bot ./cmd/bot

run:
	go run ./cmd/bot

clean:
	rm -f bot bot_data.db

tidy:
	go mod tidy

test:
	go test ./...

docker-build:
	docker build -t channel-calmdowner-bot .
