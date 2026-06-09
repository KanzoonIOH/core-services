package app

import (
	"aiac-service/internal/lib"
	"log"
	"os"
	"strings"
)

// KafkaBrokers reads and validates REDPANDA_BROKER, returning the parsed
// broker list. It exits the process if the variable is unset.
func KafkaBrokers() []string {
	broker := os.Getenv("REDPANDA_BROKER")
	if broker == "" {
		log.Fatal("REDPANDA_BROKER is required")
	}

	brokers := strings.Split(broker, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}
	return brokers
}

// Kafka builds a producer from the configured brokers.
func Kafka() *lib.KafkaProducer {
	return KafkaFromBrokers(KafkaBrokers())
}

// KafkaFromBrokers builds a producer from an already-parsed broker list,
// avoiding a second read of REDPANDA_BROKER.
func KafkaFromBrokers(brokers []string) *lib.KafkaProducer {
	producer, err := lib.NewKafkaProducer(brokers)
	if err != nil {
		log.Fatalf("kafka producer: %v", err)
	}

	return producer
}
