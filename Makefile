.PHONY: build run test tidy vet fmt up down logs migrate-up migrate-down

# golang-migrate CLI target (optional; app also auto-migrates on startup).
# Requires the `migrate` CLI: https://github.com/golang-migrate/migrate
DB_URL ?= postgres://marketd:marketd@localhost:5433/marketd?sslmode=disable

migrate-up:
	migrate -path migrations -database "$(DB_URL)" up

migrate-down:
	migrate -path migrations -database "$(DB_URL)" down 1

build:
	go build -o bin/marketd ./cmd/marketd

run:
	go run ./cmd/marketd serve

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
