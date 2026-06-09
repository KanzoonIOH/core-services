package main

import (
	"aiac-service/internal/app"
	"aiac-service/internal/timer"
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	brokers := app.KafkaBrokers()
	producer := app.KafkaFromBrokers(brokers)
	defer producer.Close()

	cfg := timer.ConfigFromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("Starting timer service (timeout=%s, poll=%s)", cfg.ConversationTimeout, cfg.PollEvery)
	if err := timer.NewService(producer, cfg).Run(ctx, brokers); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("timer service: %v", err)
	}
}
