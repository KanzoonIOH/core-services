package app

import (
	"aiac-service/internal/lib"
	"log"
	"os"
	"strings"
)

func Kafka() *lib.KafkaProducer {
	broker := os.Getenv("REDPANDA_BROKER")
	if broker == "" {
		log.Fatal("REDPANDA_BROKER is required")
	}

	brokers := strings.Split(broker, ",")

	producer, err := lib.NewKafkaProducer(brokers)
	if err != nil {
		log.Fatalf("kafka producer: %v", err)
	}

	return producer
}
