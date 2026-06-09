package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaProducer struct {
	client *kgo.Client
}

func NewKafkaProducer(brokers []string) (*KafkaProducer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ProduceRequestTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka producer: %w", err)
	}
	return &KafkaProducer{client: client}, nil
}

// Publish marshals value as JSON and produces it to topic asynchronously
// (fire-and-forget). A failed produce is only logged. Use PublishSync when the
// caller must know the record was acknowledged before proceeding (e.g. before
// committing offsets or deleting in-memory state).
func (p *KafkaProducer) Publish(ctx context.Context, topic string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("kafka marshal: %w", err)
	}

	rec := &kgo.Record{
		Topic: topic,
		Value: b,
	}

	p.client.Produce(ctx, rec, func(_ *kgo.Record, err error) {
		if err != nil {
			log.Printf("kafka produce error topic=%s: %v", rec.Topic, err)
		}
	})

	return nil
}

// PublishSync marshals value as JSON and produces it to topic, blocking until
// the broker acknowledges the record (or ctx is cancelled). It returns the
// produce error so the caller can react to delivery failures.
func (p *KafkaProducer) PublishSync(ctx context.Context, topic string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("kafka marshal: %w", err)
	}

	rec := &kgo.Record{
		Topic: topic,
		Value: b,
	}

	if err := p.client.ProduceSync(ctx, rec).FirstErr(); err != nil {
		return fmt.Errorf("kafka produce topic=%s: %w", topic, err)
	}

	return nil
}

func (p *KafkaProducer) Close() {
	p.client.Close()
}
