package main

import (
	"aic3-service/internal/app"
	"aic3-service/internal/timer"
	"context"
	"errors"
	"log"
	"log/slog"
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

	slog.Info("starting timer service", "timeout", cfg.ConversationTimeout, "poll", cfg.PollEvery)
	if err := timer.NewService(producer, cfg).Run(ctx, brokers); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("timer service: %v", err)
	}
}
