# Go + chi + sqlc

The backend. One repo, two binaries: the API (`cmd/api`) and the timer (`cmd/timer`). Do API Service only.

## How To Run Locally

Everything the service talks to already runs on the dev server. You don't need to install or start any database, you fill in the env and run.

### 1. Setup .env

Make sure env is setted up

```bash
cp .env.example .env
```

Then fill in the values.

### 2. Go

Make sure Go is installed. To check run

```bash
go version
```

Needs 1.25 or newer. Install it however you like, natively, `brew`, `asdf`, `mise`, whatever you already use.

### 3. Run the API

```bash
# Make command, use air under the hood
make dev

# hot reload using air
air

# native go
go run ./cmd/api
```

It listens on `SERVER_PORT` (6701 by default). Quick check that it is alive:

```bash
curl localhost:6701/health
```

If you want it to rebuild on save, install air once
(`go install github.com/air-verse/air@latest`) and use `make dev` instead.

### 4. Run the timer

Everything above is the API. The timer is a separate binary that handles the
scheduled jobs, and it is a separate terminal:

```bash
make timer
```

Same `.env`. You only need it when you are working on scheduled things, the
API is perfectly happy without it.

## Other commands

```bash
make help    # every target, with a short description
make test    # go test ./...
make format  # go fmt + sqlfluff on the SQL
make tidy    # go mod tidy + verify
```

## Where things are

- `cmd/api`, `cmd/timer` — the two entrypoints, thin.
- `internal/handler/` — the actual endpoints, one file per resource. There is
  no service layer on purpose, handlers hold the sqlc queries directly.
- `internal/app/`, `internal/lib/` — wiring and shared helpers.
- `internal/timer/` — the scheduled jobs.
- `db/postgres/queries/` — the SQL you write.
- `db/postgres/sqlc/` — generated from it, never edit by hand.
- `db/clickhouse/store/` — looks generated, is not. Edit it directly.
