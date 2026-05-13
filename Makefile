APP_NAME      := aiac-service
BUILD_DIR     := build
CMD_DIR       := ./cmd/api
MIGRATION_DIR := ./db/postgres/migrations

# Load environment variables from .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

.PHONY: dev build run tidy test format

dev:
	go run $(CMD_DIR)

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

run: build
	./$(BUILD_DIR)/$(APP_NAME)

tidy:
	go mod tidy
	go mod verify

test:
	go test ./...

format:
	go fmt ./cmd/... ./internal/...
	sqlfluff format ./db

.PHONY: goose-pg-new goose-pg-up goose-pg-down goose-pg-status goose-pg-validate

goose-pg-new:
	goose -dir $(MIGRATION_DIR) create $(name) sql

goose-pg-up:
	goose -dir $(MIGRATION_DIR) postgres "$(DB_URL)" up

goose-pg-down:
	goose -dir $(MIGRATION_DIR) postgres "$(DB_URL)" down

goose-pg-status:
	goose -dir $(MIGRATION_DIR) postgres "$(DB_URL)" status

goose-pg-validate:
	goose -dir $(MIGRATION_DIR) validate

.PHONY: db-start db-stop db-reset

db-start:
	@echo "Starting PostgreSQL database..."
	docker run -d \
		--name $(DB_CONTAINER) \
		-e POSTGRES_USER=$(DB_USER) \
		-e POSTGRES_PASSWORD=$(DB_PASSWORD) \
		-e POSTGRES_DB=$(DB_NAME) \
		-p $(DB_PORT):5432 \
		postgres:18 \
		|| (echo "Database container already exists, starting it..." && docker start $(DB_CONTAINER))
	@echo "Waiting for database to be ready..."
	@sleep 3
	@echo "Database is ready!"

db-stop:
	@echo "Stopping PostgreSQL database..."
	docker stop $(DB_CONTAINER) || echo "Database container is not running"

db-reset:
	@echo "Resetting PostgreSQL database..."
	docker stop $(DB_CONTAINER) || true
	docker rm $(DB_CONTAINER) || true
	@echo "Starting fresh PostgreSQL database..."
	docker run -d \
		--name $(DB_CONTAINER) \
		-e POSTGRES_USER=$(DB_USER) \
		-e POSTGRES_PASSWORD=$(DB_PASSWORD) \
		-e POSTGRES_DB=$(DB_NAME) \
		-p $(DB_PORT):5432 \
		postgres:18
	@echo "Waiting for database to be ready..."
	@sleep 5
	@echo "Database is ready! Running migrations..."
	$(MAKE) goose-pg-up
