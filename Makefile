.PHONY: help proto deps-up deps-down migrate-up migrate-down seed \
        test-unit test-integration test-e2e test-coverage lint \
        build run-all

DB_URL=postgres://postgres:secret@localhost:5432/parkirpintar?sslmode=disable

help:
	@echo "ParkirPintar — Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | awk 'BEGIN{FS=":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

proto: ## Generate Go code from proto files
	cd proto && buf dep update && buf generate

deps-up: ## Start local dependencies (postgres, redis, rabbitmq)
	docker-compose up -d
	@echo "Waiting for services to be healthy..."
	@sleep 3

deps-down: ## Stop local dependencies
	docker-compose down

migrate-up: ## Run all database migrations
	migrate -path migrations/ -database "$(DB_URL)" up

migrate-down: ## Rollback last migration
	migrate -path migrations/ -database "$(DB_URL)" down 1

migrate-reset: ## Rollback all migrations
	migrate -path migrations/ -database "$(DB_URL)" drop -f

seed: ## Seed database with initial spot data
	go run scripts/seed/main.go

test-unit: ## Run unit tests
	go test ./pkg/... -v -count=1 -race

test-integration: ## Run integration tests (requires docker)
	go test ./tests/integration/... -v -count=1 -tags integration

test-e2e: ## Run end-to-end tests
	go test ./tests/e2e/... -v -count=1 -tags e2e

test-coverage: ## Run all tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: ## Run linter
	golangci-lint run ./...

build: ## Build all service binaries
	@for svc in gateway reservation billing payment presence notification; do \
		echo "Building $$svc..."; \
		go build -o bin/$$svc ./services/$$svc/cmd; \
	done

run-gateway: ## Run gateway service
	go run ./services/gateway/cmd/main.go

run-reservation: ## Run reservation service
	go run ./services/reservation/cmd/main.go

run-billing: ## Run billing service
	go run ./services/billing/cmd/main.go

run-payment: ## Run payment service
	go run ./services/payment/cmd/main.go

run-presence: ## Run presence service
	go run ./services/presence/cmd/main.go

run-notification: ## Run notification service
	go run ./services/notification/cmd/main.go
