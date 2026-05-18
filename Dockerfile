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

# Copy .env so godotenv.Load() can pick it up at runtime
COPY .env .

EXPOSE 6767

CMD ["./server"]
