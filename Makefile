# --- Configuration -----------------------------------------------------------
APP_NAME       ?= groups-control
BIN_DIR        ?= bin
SERVER_BIN     ?= $(BIN_DIR)/server

# Database / migrations
DB_DSN         ?= postgres://postgres:postgres@localhost:5432/groups?sslmode=disable
MIGRATIONS_DIR ?= migrations

# Compose
COMPOSE_FILE   ?= deployments/docker-compose.yml

# Tools
OAPI_CODEGEN   ?= go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen
MIGRATE        ?= go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate
GOLANGCI_LINT  ?= go run github.com/golangci/golangci-lint/cmd/golangci-lint

.DEFAULT_GOAL := help

# --- Help --------------------------------------------------------------------
.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# --- Code generation ---------------------------------------------------------
.PHONY: gen
gen: ## Generate API types/server from OpenAPI (oapi-codegen)
	$(OAPI_CODEGEN) -config oapi-codegen.yaml api/openapi.yaml

# --- Build / run -------------------------------------------------------------
.PHONY: build
build: ## Build the server binary
	go build -o $(SERVER_BIN) ./cmd/server

.PHONY: run
run: ## Run the server locally
	go run ./cmd/server

.PHONY: tidy
tidy: ## go mod tidy
	go mod tidy

# --- Migrations --------------------------------------------------------------
.PHONY: migrate-up
migrate-up: ## Apply all up migrations
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	$(MIGRATE) -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" down 1

.PHONY: migrate-create
migrate-create: ## Create a new migration: make migrate-create name=foo
	$(MIGRATE) create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

# --- Quality -----------------------------------------------------------------
.PHONY: test
test: ## Run all tests
	go test ./... -race -count=1

.PHONY: lint
lint: ## Run golangci-lint
	$(GOLANGCI_LINT) run

.PHONY: fmt
fmt: ## Format code
	go fmt ./...

# --- Docker compose ----------------------------------------------------------
.PHONY: compose-up
compose-up: ## Start the full stack (db + migrate + app)
	docker compose -f $(COMPOSE_FILE) up --build -d

.PHONY: compose-down
compose-down: ## Stop the stack and remove volumes
	docker compose -f $(COMPOSE_FILE) down -v
