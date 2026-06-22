package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"aic3-service/internal/app"
	"aic3-service/internal/handler"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		log.Fatal("SERVER_PORT is required")
	}

	// Postgres
	conn, err := app.DB()
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	// Redpanda / Kafka producer
	kafka := app.Kafka()
	defer kafka.Close()

	// ClickHouse
	ch := app.ClickHouse()
	defer ch.Close()

	// Background consumer: Redpanda → ClickHouse
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	brokers := strings.Split(os.Getenv("REDPANDA_BROKER"), ",")
	handler.StartChatConsumer(ctx, brokers, ch)

	r := app.AppRouter(conn, kafka, ch)

	log.Printf("Starting server on :%v", serverPort)
	if err := http.ListenAndServe(":"+serverPort, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
