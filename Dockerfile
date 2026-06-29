# ── Build stage ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Download dependencies first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/api

# ── Runtime stage ───────────────────────────────────────────────────────────────
FROM alpine:3.21

WORKDIR /app

# Copy the compiled binary from the builder
COPY --from=builder /app/server .

# No .env is baked into the image. Configuration is supplied at runtime via
# docker-compose `env_file:`/`environment:`. godotenv.Load() simply no-ops when
# no ./.env exists, and never overrides already-set env vars.

EXPOSE 6701

CMD ["./server"]
