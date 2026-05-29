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
			_ = err
		}
	})

	return nil
}

func (p *KafkaProducer) Close() {
	p.client.Close()
}
