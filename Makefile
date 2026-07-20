.PHONY: build run test test-integration test-e2e test-report tidy vet fmt up down logs migrate-up migrate-down swagger

# golang-migrate CLI target (optional; app also auto-migrates on startup).
# Requires the `migrate` CLI: https://github.com/golang-migrate/migrate
DB_URL ?= postgres://marketd:marketd@localhost:5433/marketd?sslmode=disable

# DB connection env for tests that need Postgres (integration + e2e).
IT_ENV = DB_HOST=localhost DB_PORT=5433 DB_USER=marketd DB_PASSWORD=marketd DB_NAME=marketd DB_SSLMODE=disable

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

# Integration tests require a running Postgres (see docker compose / .env).
test-integration:
	$(IT_ENV) go test -tags integration -race ./...

# HTTP end-to-end tests: drive the full router over the real database.
test-e2e:
	$(IT_ENV) go test -tags e2e -race ./test/e2e/...

# Run unit + integration + e2e tests and write a visual HTML dashboard to
# test-report.html. Open it in a browser to see per-package, per-test results.
test-report:
	$(IT_ENV) go test -json -tags 'integration e2e' ./... | go run ./tools/testreport -t "Market Dragon Tests" -o test-report.html

# Regenerate OpenAPI docs (docs/swagger.yaml, docs/swagger.json, docs/docs.go)
# from the swaggo annotations in the handlers.
swagger:
	go run github.com/swaggo/swag/cmd/swag@latest init -g cmd/marketd/main.go -o docs --parseInternal --parseDepth 2

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
