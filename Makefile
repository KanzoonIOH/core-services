APP_NAME         := aic3-service
TIMER_APP_NAME   := aic3-timer-service
BUILD_DIR        := build
CMD_DIR          := ./cmd/api
TIMER_CMD_DIR    := ./cmd/timer
MIGRATION_DIR    := ./db/postgres/migrations
CH_MIGRATION_DIR := ./db/clickhouse/migrations

# Orchestrator .env supplies DB_USER/DB_PASSWORD_ENCODED/DB_NAME for the
# compose-goose-* targets. Included FIRST so local .env wins on any overlap
# (CLICKHOUSE_PORT differs: 6761 native here vs 6715 HTTP there).
# ifneq (,$(wildcard ../compose-orchestrator/.env))
#     include ../compose-orchestrator/.env
# endif

# Load environment variables from .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# air with hot reload. ENV picks the env file: `make dev` -> .env,
# `make dev ENV=local` -> .env.local, `make dev ENV=staging` -> .env.staging.
# ENV ?=
ENV_FILE = $(if $(ENV),.env.$(ENV),.env)

# Image build + push for k8s (registry must match k8s image: fields).
# Override: make image-push REGISTRY=10.10.1.122/agent TAG=v1
REGISTRY   ?= 10.10.1.122/agent
TAG        ?= latest
API_IMG    := $(REGISTRY)/aic3-api:$(TAG)
TIMER_IMG  := $(REGISTRY)/aic3-timer:$(TAG)
MIGRATE_IMG := $(REGISTRY)/aic3-migrations:$(TAG)
API_TAR    := aic3-api-$(TAG).tar.gz
TIMER_TAR  := aic3-timer-$(TAG).tar.gz

.PHONY: help dev timer build build-timer run tidy test format
.PHONY: image image-timer image-migrations image-push image-timer-push image-migrations-push release-images
.PHONY: image-save image-timer-save image-load image-timer-load

##@ General
help: ## Show this help
	@awk 'BEGIN{FS=":.*?## "} /^##@ /{printf "\n%s\n", substr($$0,5)} /^[a-z][a-z0-9-]+:.*?## /{printf "  %-23s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Development
dev: ## Run api with air hot reload (ENV=local -> .env.local)
	air -env_files $(ENV_FILE)

timer: ## Run the timer service
	go run $(TIMER_CMD_DIR)

build: ## Build the api binary
	go build -o $(BUILD_DIR)/$(APP_NAME) $(CMD_DIR)

build-timer: ## Build the timer binary
	go build -o $(BUILD_DIR)/$(TIMER_APP_NAME) $(TIMER_CMD_DIR)

run: build ## Build then run the api binary
	./$(BUILD_DIR)/$(APP_NAME)

##@ Images (k8s: build then push to the registry k8s pulls from)
image: ## Build the api image
	docker build -f Dockerfile -t $(API_IMG) .

image-timer: ## Build the timer image
	docker build -f Dockerfile.timer -t $(TIMER_IMG) .

image-push: image ## Build + push the api image
	docker push $(API_IMG)

image-timer-push: image-timer ## Build + push the timer image
	docker push $(TIMER_IMG)

image-migrations: ## Build the goose migrations image (k8s migrate Job)
	docker build -f Dockerfile.migrations -t $(MIGRATE_IMG) .

image-migrations-push: image-migrations ## Build + push the migrations image
	docker push $(MIGRATE_IMG)

release-images: image-push image-timer-push image-migrations-push ## Build + push all three images

# Tar flow (when you can't push): save here, copy the .tar.gz to the server, load there.
image-save: image ## Save the api image to a .tar.gz
	docker save $(API_IMG) | gzip > $(API_TAR)
	@echo "Wrote $(API_TAR) -- copy to server, then 'make image-load' there"

image-timer-save: image-timer ## Save the timer image to a .tar.gz
	docker save $(TIMER_IMG) | gzip > $(TIMER_TAR)
	@echo "Wrote $(TIMER_TAR) -- copy to server, then 'make image-timer-load' there"

image-load: ## Load the api image from its .tar.gz (run ON the server)
	gunzip < $(API_TAR) | docker load

image-timer-load: ## Load the timer image from its .tar.gz (run ON the server)
	gunzip < $(TIMER_TAR) | docker load

tidy: ## go mod tidy + verify
	go mod tidy
	go mod verify

test: ## Run all tests
	go test ./...

format: ## go fmt + sqlfluff format
	go fmt ./cmd/... ./internal/...
	sqlfluff format ./db/postgres

.PHONY: goose-pg-new goose-pg-up goose-pg-down goose-pg-status goose-pg-validate

##@ Migrations — Postgres (local, DB_URL)
goose-pg-new: ## Create a new postgres migration (name=...)
	goose -dir $(MIGRATION_DIR) create $(name) sql

goose-pg-up: ## Migrate postgres up
	goose -dir $(MIGRATION_DIR) postgres "$(DB_URL)" up

goose-pg-down: ## Migrate postgres down one step
	goose -dir $(MIGRATION_DIR) postgres "$(DB_URL)" down

goose-pg-status: ## Show postgres migration status
	goose -dir $(MIGRATION_DIR) postgres "$(DB_URL)" status

goose-pg-validate: ## Validate postgres migrations
	goose -dir $(MIGRATION_DIR) validate

.PHONY: db-start db-stop db-reset

##@ Local Postgres container
db-start: ## Start (or create) the local postgres container
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

db-stop: ## Stop the local postgres container
	@echo "Stopping PostgreSQL database..."
	docker stop $(DB_CONTAINER) || echo "Database container is not running"

db-reset: ## Recreate the local postgres container + run migrations
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

##@ Migrations — ClickHouse (local, CLICKHOUSE_URL)
goose-ch-new: ## Create a new clickhouse migration (name=...)
	goose -dir $(CH_MIGRATION_DIR) create $(name) sql

goose-ch-up: ## Migrate clickhouse up
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" up

goose-ch-down: ## Migrate clickhouse down one step
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" down

goose-ch-status: ## Show clickhouse migration status
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" status

goose-ch-validate: ## Validate clickhouse migrations
	goose -dir $(CH_MIGRATION_DIR) validate

goose-ch-reset: ## Reset (drop all) clickhouse migrations
	goose -dir $(CH_MIGRATION_DIR) clickhouse "$(CLICKHOUSE_URL)" reset

# ─── Goose for docker-compose (internal hostnames, root .env creds) ────────────
#
# Runs goose INSIDE the compose network via a throwaway container, reaching the
# DBs by service name (postgres:5432, clickhouse:9000). Creds come from the ROOT
# .env (the single source of truth) — the connection URLs are assembled here from
# its parts, exactly as the root compose does. No host-port publishing needed.
#
# Prereqs: stack running (`make deps` at repo root). Run from repo root via
# `make migrate`, or from ./core-services directly.
#
# Override the compose project name if you used `-p` / a custom `name:`.
COMPOSE_PROJECT  ?= aic3
COMPOSE_NETWORK  ?= $(COMPOSE_PROJECT)_aic3-net
GOOSE_IMAGE      ?= ghcr.io/kukymbr/goose-docker:3.24.1

# (orchestrator .env is included at the top of this file)
# ifneq (,$(wildcard ../compose-orchestrator/.env))
#     include ../compose-orchestrator/.env
#     export
# endif

# Internal-hostname connection strings (NOT the host-published ports).
COMPOSE_DB_URL := postgres://$(DB_USER):$(DB_PASSWORD_ENCODED)@postgres:5432/$(DB_NAME)?sslmode=disable
COMPOSE_CH_URL := clickhouse://clickhouse:9000/$(CLICKHOUSE_DATABASE)?username=$(CLICKHOUSE_USER)&password=$(CLICKHOUSE_PASSWORD)

# Run goose in a one-off container on the compose network.
# $(1) = goose dialect (postgres|clickhouse)
# $(2) = DBSTRING connection url
# $(3) = migrations dir (mounted read-only)
# $(4) = goose command (up|down|status|reset|...)
define compose_goose
	docker run --rm \
		--network $(COMPOSE_NETWORK) \
		-e GOOSE_DRIVER=$(1) \
		-e GOOSE_DBSTRING="$(2)" \
		-e GOOSE_MIGRATION_DIR=/migrations \
		-v "$(CURDIR)/$(3):/migrations:ro" \
		$(GOOSE_IMAGE) $(4)
endef

.PHONY: compose-goose-pg-up compose-goose-pg-down compose-goose-pg-status compose-goose-pg-reset
.PHONY: compose-goose-ch-up compose-goose-ch-down compose-goose-ch-status compose-goose-ch-reset
.PHONY: compose-migrate

##@ Migrations — compose network (internal hostnames, root .env creds)
# Postgres (internal: postgres:5432)
compose-goose-pg-up: ## Migrate postgres up (compose network)
	$(call compose_goose,postgres,$(COMPOSE_DB_URL),$(MIGRATION_DIR),up)

compose-goose-pg-down: ## Migrate postgres down one step (compose network)
	$(call compose_goose,postgres,$(COMPOSE_DB_URL),$(MIGRATION_DIR),down)

compose-goose-pg-status: ## Show postgres migration status (compose network)
	$(call compose_goose,postgres,$(COMPOSE_DB_URL),$(MIGRATION_DIR),status)

compose-goose-pg-reset: ## Reset postgres migrations (compose network)
	$(call compose_goose,postgres,$(COMPOSE_DB_URL),$(MIGRATION_DIR),reset)

# ClickHouse (internal: clickhouse:9000)
compose-goose-ch-up: ## Migrate clickhouse up (compose network)
	$(call compose_goose,clickhouse,$(COMPOSE_CH_URL),$(CH_MIGRATION_DIR),up)

compose-goose-ch-down: ## Migrate clickhouse down one step (compose network)
	$(call compose_goose,clickhouse,$(COMPOSE_CH_URL),$(CH_MIGRATION_DIR),down)

compose-goose-ch-status: ## Show clickhouse migration status (compose network)
	$(call compose_goose,clickhouse,$(COMPOSE_CH_URL),$(CH_MIGRATION_DIR),status)

compose-goose-ch-reset: ## Reset clickhouse migrations (compose network)
	$(call compose_goose,clickhouse,$(COMPOSE_CH_URL),$(CH_MIGRATION_DIR),reset)

# Run both Postgres + ClickHouse migrations up (typical post-`up` step).
compose-migrate: compose-goose-pg-up compose-goose-ch-up ## Run all migrations up (compose network)
	@echo "All migrations applied."

copy-api:
	rsync -avPR $(API_TAR) root@aiplatform2:/home/ubuntu/aic3/builds
