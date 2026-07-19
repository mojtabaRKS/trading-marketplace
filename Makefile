.PHONY: build run test tidy vet fmt up down logs

build:
	go build -o bin/marketd ./cmd/marketd

run:
	go run ./cmd/marketd

test:
	go test -race ./...

tidy:
	go mod tidy

vet:
	go vet ./...

fmt:
	gofmt -s -w .

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f app
