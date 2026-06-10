package timer

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Config holds the tunable parameters of the timer service. Every field can be
// overridden via an environment variable; unset variables fall back to the
// documented defaults.
type Config struct {
	// ConversationTimeout is how long a conversation may stay idle before it is
	// closed with end_reason="timed_out". Env: TIMER_CONVERSATION_TIMEOUT.
	ConversationTimeout time.Duration

	// PollEvery is the maximum time a single Kafka poll blocks. Env: TIMER_POLL_INTERVAL.
	PollEvery time.Duration

	// EventBuffer is the size of the batch channel between the consumer goroutine
	// and the event loop. Env: TIMER_EVENT_BUFFER.
	EventBuffer int

	// PollBatch is the maximum number of records fetched per poll. Env: TIMER_POLL_BATCH.
	PollBatch int

	// DedupeTTL is how long a conversation_id is remembered after it ends so that
	// a racing timeout/explicit-end pair does not emit duplicate end events.
	// Env: TIMER_DEDUPE_TTL.
	DedupeTTL time.Duration
}

// ConfigFromEnv builds a Config from the environment, applying defaults.
func ConfigFromEnv() Config {
	return Config{
		ConversationTimeout: envDuration("TIMER_CONVERSATION_TIMEOUT", 10*time.Second),
		PollEvery:           envDuration("TIMER_POLL_INTERVAL", 500*time.Millisecond),
		EventBuffer:         envInt("TIMER_EVENT_BUFFER", 256),
		PollBatch:           envInt("TIMER_POLL_BATCH", 100),
		DedupeTTL:           envDuration("TIMER_DEDUPE_TTL", 10*time.Minute),
	}
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Printf("timer config: invalid %s=%q, using default %s", key, v, def)
		return def
	}
	return d
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Printf("timer config: invalid %s=%q, using default %d", key, v, def)
		return def
	}
	return n
}
