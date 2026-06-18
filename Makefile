APP_NAME         := aiac-service
TIMER_APP_NAME   := aiac-timer-service
BUILD_DIR        := build
CMD_DIR          := ./cmd/api
TIMER_CMD_DIR    := ./cmd/timer
MIGRATION_DIR    := ./db/postgres/migrations
CH_MIGRATION_DIR := ./db/clickhouse/migrations

# Load environment variables from .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

.PHONY: dev timer build build-timer run tidy test format

dev:
	go run $(CMD_DIR)

timer:
	go run $(TIMER_CMD_DIR)

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

build-timer:
	go build -o $(BUILD_DIR)/$(TIMER_APP_NAME) $(TIMER_CMD_DIR)

run: build
	./$(BUILD_DIR)/$(APP_NAME)

tidy:
	go mod tidy
	go mod verify

test:
	go test ./...

format:
	go fmt ./cmd/... ./internal/...
	sqlfluff format ./db/postgres

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

# ─── ClickHouse ───────────────────────────────────────────────────────────────

.PHONY: goose-ch-new goose-ch-up goose-ch-down goose-ch-status goose-ch-validate goose-ch-reset

goose-ch-new:
	goose -dir $(CH_MIGRATION_DIR) create $(name) sql

goose-ch-up:
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" up

goose-ch-down:
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" down

goose-ch-status:
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" status

goose-ch-validate:
	goose -dir $(CH_MIGRATION_DIR) validate

goose-ch-reset:
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" reset

# ─── Goose for docker-compose (uses service/.env.compose, internal hostnames) ──
#
# These targets run goose INSIDE the compose network (aiac-net) via a throwaway
# container, so they reach the DBs by their internal service names
# (postgres:5432, clickhouse:9000) using the creds from service/.env.compose.
# No host-port publishing required — works even though ClickHouse is internal-only.
#
# Prereqs: the stack must be running (`docker compose up`) so the network and DBs
# exist. Run these from the repo ROOT? No — from ./service (where this Makefile
# and the migration dirs live).
#
# Override the compose project name if you used `-p` / a custom `name:`.
COMPOSE_PROJECT  ?= aiac
COMPOSE_NETWORK  ?= $(COMPOSE_PROJECT)_aiac-net
COMPOSE_ENV_FILE ?= .env.compose
GOOSE_IMAGE      ?= ghcr.io/kukymbr/goose-docker:3.24.1

# Run goose in a one-off container on the compose network.
# $(1) = goose dialect (postgres|clickhouse)
# $(2) = DBSTRING env var name to read from .env.compose (DB_URL|CLICKHOUSE_URL)
# $(3) = migrations dir (mounted read-only)
# $(4) = goose command (up|down|status|reset|...)
define compose_goose
	docker run --rm \
		--network $(COMPOSE_NETWORK) \
		--env-file $(COMPOSE_ENV_FILE) \
		-e GOOSE_DRIVER=$(1) \
		-e GOOSE_DBSTRING="$$$(2)" \
		-e GOOSE_MIGRATION_DIR=/migrations \
		-v "$(CURDIR)/$(3):/migrations:ro" \
		$(GOOSE_IMAGE) $(4)
endef

.PHONY: compose-goose-pg-up compose-goose-pg-down compose-goose-pg-status compose-goose-pg-reset
.PHONY: compose-goose-ch-up compose-goose-ch-down compose-goose-ch-status compose-goose-ch-reset
.PHONY: compose-migrate

# Postgres (reads DB_URL from .env.compose -> postgres:5432)
compose-goose-pg-up:
	$(call compose_goose,postgres,DB_URL,$(MIGRATION_DIR),up)

compose-goose-pg-down:
	$(call compose_goose,postgres,DB_URL,$(MIGRATION_DIR),down)

compose-goose-pg-status:
	$(call compose_goose,postgres,DB_URL,$(MIGRATION_DIR),status)

compose-goose-pg-reset:
	$(call compose_goose,postgres,DB_URL,$(MIGRATION_DIR),reset)

# ClickHouse (reads CLICKHOUSE_URL from .env.compose -> clickhouse:9000)
compose-goose-ch-up:
	$(call compose_goose,clickhouse,CLICKHOUSE_URL,$(CH_MIGRATION_DIR),up)

compose-goose-ch-down:
	$(call compose_goose,clickhouse,CLICKHOUSE_URL,$(CH_MIGRATION_DIR),down)

compose-goose-ch-status:
	$(call compose_goose,clickhouse,CLICKHOUSE_URL,$(CH_MIGRATION_DIR),status)

compose-goose-ch-reset:
	$(call compose_goose,clickhouse,CLICKHOUSE_URL,$(CH_MIGRATION_DIR),reset)

# Run both Postgres + ClickHouse migrations up (typical post-`up` step).
compose-migrate: compose-goose-pg-up compose-goose-ch-up
	@echo "All migrations applied."
