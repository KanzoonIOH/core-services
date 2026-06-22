package timer

import (
	"aic3-service/internal/handler"
	"aic3-service/internal/lib"
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const consumerGroup = "aic3-timer-service"

type activityEvent struct {
	AgentID        string    `json:"agent_id"`
	ConversationID string    `json:"conversation_id"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type conversationEndEvent struct {
	ConversationID string `json:"conversation_id"`
}

// recordRef locates a Kafka record so its offset can be gated/committed.
type recordRef struct {
	topic     string
	partition int32
	offset    int64
}

type timerEvent struct {
	activity *activityEvent
	endedID  string
	ref      recordRef
}

// batch carries a decoded set of events together with the highest offset seen
// per partition and a commit function. The event loop processes the events,
// fires due timers synchronously, then computes a safe commit point that never
// advances past an activity record whose timer is still pending. This lets a
// restart replay unresolved activity and rebuild the in-memory heap.
type batch struct {
	events  []timerEvent
	maxSeen map[partitionKey]int64
	commit  func(map[partitionKey]int64)
}

type partitionKey struct {
	topic     string
	partition int32
}

type entry struct {
	agentID        string
	conversationID string
	startedAt      time.Time
	lastActivityAt time.Time
	fireAt         time.Time
	ref            recordRef // activity record that currently backs this timer
	index          int
}

type entryHeap []*entry

func (h entryHeap) Len() int { return len(h) }

func (h entryHeap) Less(i, j int) bool { return h[i].fireAt.Before(h[j].fireAt) }

func (h entryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *entryHeap) Push(x any) {
	e := x.(*entry)
	e.index = len(*h)
	*h = append(*h, e)
}

func (h *entryHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	e.index = -1
	*h = old[:n-1]
	return e
}

// metrics tracks operational counters reported periodically.
type metrics struct {
	activityEvents atomic.Int64
	endEvents      atomic.Int64
	firedTimeouts  atomic.Int64
	publishErrors  atomic.Int64
	dropped        atomic.Int64 // events skipped by dedupe
}

// Service consumes conversation activity and closes inactive conversations
// after cfg.ConversationTimeout. Timer state is kept in memory; pending timers
// survive a restart because activity offsets are not committed until their
// timer resolves (per-record offset gating).
type Service struct {
	producer *lib.KafkaProducer
	cfg      Config
	metrics  metrics
}

func NewService(producer *lib.KafkaProducer, cfg Config) *Service {
	return &Service{producer: producer, cfg: cfg}
}

func (s *Service) Run(ctx context.Context, brokers []string) error {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(consumerGroup),
		kgo.ConsumeTopics(handler.TopicConversationActivity, handler.TopicConversationEnd),
		// Offsets are committed manually and gated behind unresolved timers so a
		// restart replays pending activity (see commit logic below).
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return err
	}
	defer client.Close()

	batches := make(chan batch, s.cfg.EventBuffer)
	go s.consume(ctx, client, batches)

	entries := make(map[string]*entry)
	timers := &entryHeap{}
	heap.Init(timers)

	// ended tracks conversation_ids resolved (timed out or explicitly ended)
	// recently, so a racing duplicate does not emit a second end event.
	ended := make(map[string]time.Time)

	var next <-chan time.Time
	var wake *time.Timer
	resetWake := func() {
		if wake != nil {
			if !wake.Stop() {
				select {
				case <-wake.C:
				default:
				}
			}
		}
		if timers.Len() == 0 {
			next = nil
			return
		}
		d := time.Until((*timers)[0].fireAt)
		if d < 0 {
			d = 0
		}
		wake = time.NewTimer(d)
		next = wake.C
	}
	defer func() {
		if wake != nil {
			wake.Stop()
		}
	}()

	resetWake()

	reportTicker := time.NewTicker(time.Minute)
	defer reportTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case b := <-batches:
			for _, evt := range b.events {
				if evt.activity != nil {
					s.metrics.activityEvents.Add(1)
					if isEnded(ended, evt.activity.ConversationID, s.cfg.DedupeTTL) {
						s.metrics.dropped.Add(1)
					} else {
						upsertTimer(entries, timers, *evt.activity, evt.ref, s.cfg.ConversationTimeout)
					}
				} else if evt.endedID != "" {
					s.metrics.endEvents.Add(1)
					removeTimer(entries, timers, evt.endedID)
					ended[evt.endedID] = time.Now()
				}
			}
			// Fire anything due before committing so resolved timers are durably
			// published; entries that fail to publish stay in the heap for retry.
			s.fireDue(ctx, entries, timers, ended)
			pruneEnded(ended, s.cfg.DedupeTTL)
			if b.commit != nil {
				b.commit(safeCommitOffsets(b.maxSeen, timers))
			}
			resetWake()
		case <-next:
			s.fireDue(ctx, entries, timers, ended)
			pruneEnded(ended, s.cfg.DedupeTTL)
			resetWake()
		case <-reportTicker.C:
			s.report(timers.Len(), len(ended))
		}
	}
}

func (s *Service) consume(ctx context.Context, client *kgo.Client, batches chan<- batch) {
	for {
		pollCtx, cancel := context.WithTimeout(ctx, s.cfg.PollEvery)
		fetches := client.PollRecords(pollCtx, s.cfg.PollBatch)
		cancel()

		if fetches.IsClientClosed() || ctx.Err() != nil {
			return
		}

		fetches.EachError(func(_ string, _ int32, err error) {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Printf("timer service fetch error: %v", err)
		})

		events := make([]timerEvent, 0, s.cfg.PollBatch)
		maxSeen := make(map[partitionKey]int64)
		fetches.EachRecord(func(rec *kgo.Record) {
			pk := partitionKey{topic: rec.Topic, partition: rec.Partition}
			if cur, ok := maxSeen[pk]; !ok || rec.Offset > cur {
				maxSeen[pk] = rec.Offset
			}
			ref := recordRef{topic: rec.Topic, partition: rec.Partition, offset: rec.Offset}

			switch rec.Topic {
			case handler.TopicConversationActivity:
				var evt activityEvent
				if err := json.Unmarshal(rec.Value, &evt); err != nil {
					log.Printf("timer service unmarshal activity: %v", err)
					return
				}
				if evt.AgentID == "" || evt.ConversationID == "" || evt.OccurredAt.IsZero() {
					return
				}
				events = append(events, timerEvent{activity: &evt, ref: ref})
			case handler.TopicConversationEnd:
				var evt conversationEndEvent
				if err := json.Unmarshal(rec.Value, &evt); err != nil {
					log.Printf("timer service unmarshal conversation end: %v", err)
					return
				}
				if evt.ConversationID != "" {
					events = append(events, timerEvent{endedID: evt.ConversationID, ref: ref})
				}
			}
		})

		if len(maxSeen) == 0 {
			continue
		}

		commit := func(offsets map[partitionKey]int64) {
			if len(offsets) == 0 {
				return
			}
			// CommitRecords commits record.Offset+1, so to commit up to (and not
			// including) offset N we pass a synthetic record at N-1.
			recs := make([]*kgo.Record, 0, len(offsets))
			for pk, off := range offsets {
				if off <= 0 {
					continue
				}
				recs = append(recs, &kgo.Record{
					Topic:     pk.topic,
					Partition: pk.partition,
					Offset:    off - 1,
				})
			}
			if len(recs) == 0 {
				return
			}
			if err := client.CommitRecords(ctx, recs...); err != nil &&
				!errors.Is(err, context.Canceled) {
				log.Printf("timer service commit offsets: %v", err)
			}
		}

		select {
		case batches <- batch{events: events, maxSeen: maxSeen, commit: commit}:
		case <-ctx.Done():
			return
		}
	}
}

// safeCommitOffsets computes, per partition, the next offset to commit. For the
// activity topic it never advances past the oldest still-pending timer's offset,
// so unresolved timers are replayed on restart. Other partitions (end topic)
// commit fully up to maxSeen+1.
func safeCommitOffsets(maxSeen map[partitionKey]int64, timers *entryHeap) map[partitionKey]int64 {
	// Oldest pending activity offset per partition.
	pendingMin := make(map[partitionKey]int64)
	for _, e := range *timers {
		pk := partitionKey{topic: e.ref.topic, partition: e.ref.partition}
		if cur, ok := pendingMin[pk]; !ok || e.ref.offset < cur {
			pendingMin[pk] = e.ref.offset
		}
	}

	out := make(map[partitionKey]int64, len(maxSeen))
	for pk, max := range maxSeen {
		commitAt := max + 1
		if min, ok := pendingMin[pk]; ok && min < commitAt {
			// Commit only up to (but not including) the oldest pending record.
			commitAt = min
		}
		if commitAt < 0 {
			commitAt = 0
		}
		out[pk] = commitAt
	}
	return out
}

func upsertTimer(entries map[string]*entry, timers *entryHeap, evt activityEvent, ref recordRef, timeout time.Duration) {
	fireAt := evt.OccurredAt.Add(timeout)
	if e, ok := entries[evt.ConversationID]; ok {
		e.agentID = evt.AgentID
		e.lastActivityAt = evt.OccurredAt
		e.fireAt = fireAt
		e.ref = ref
		heap.Fix(timers, e.index)
		return
	}

	e := &entry{
		agentID:        evt.AgentID,
		conversationID: evt.ConversationID,
		startedAt:      evt.OccurredAt,
		lastActivityAt: evt.OccurredAt,
		fireAt:         fireAt,
		ref:            ref,
	}
	entries[evt.ConversationID] = e
	heap.Push(timers, e)
}

func removeTimer(entries map[string]*entry, timers *entryHeap, conversationID string) {
	e, ok := entries[conversationID]
	if !ok {
		return
	}
	heap.Remove(timers, e.index)
	delete(entries, conversationID)
}

func (s *Service) fireDue(ctx context.Context, entries map[string]*entry, timers *entryHeap, ended map[string]time.Time) {
	now := time.Now().UTC()
	for timers.Len() > 0 {
		e := (*timers)[0]
		if e.fireAt.After(now) {
			return
		}

		// Publish synchronously before removing the entry. If the publish fails,
		// keep the entry in place so it is retried on the next wake; this avoids
		// silently dropping a timeout.
		if err := s.publishTimeout(ctx, e); err != nil {
			s.metrics.publishErrors.Add(1)
			log.Printf("timer service publish timeout (will retry) session=%s: %v", e.conversationID, err)
			return
		}

		heap.Pop(timers)
		delete(entries, e.conversationID)
		ended[e.conversationID] = now
		s.metrics.firedTimeouts.Add(1)
	}
}

func (s *Service) publishTimeout(ctx context.Context, e *entry) error {
	// ended_at is the moment the conversation actually timed out (fireAt), not
	// the wall-clock time the event is emitted, so resolution_ms is deterministic
	// and independent of poll/scheduling jitter.
	endedAt := e.fireAt.UTC()
	resolutionMs := endedAt.Sub(e.startedAt).Milliseconds()
	if resolutionMs < 0 {
		resolutionMs = 0
	}

	event := map[string]any{
		"agent_id":          e.agentID,
		"conversation_id":   e.conversationID,
		"end_reason":        handler.EndReasonTimedOut,
		"escalation_reason": "",
		"started_at":        e.startedAt,
		"ended_at":          endedAt,
		"resolution_ms":     resolutionMs,
		"occurred_at":       endedAt,
	}

	if err := s.producer.PublishSync(ctx, handler.TopicConversationEnd, event); err != nil {
		return err
	}
	log.Printf("timer service: timed out conversation session=%s agent=%s", e.conversationID, e.agentID)
	return nil
}

// isEnded reports whether conversationID was resolved within ttl.
func isEnded(ended map[string]time.Time, conversationID string, ttl time.Duration) bool {
	t, ok := ended[conversationID]
	if !ok {
		return false
	}
	if time.Since(t) > ttl {
		delete(ended, conversationID)
		return false
	}
	return true
}

// pruneEnded drops dedupe entries older than ttl to bound memory.
func pruneEnded(ended map[string]time.Time, ttl time.Duration) {
	for id, t := range ended {
		if time.Since(t) > ttl {
			delete(ended, id)
		}
	}
}

func (s *Service) report(activeTimers, dedupeTracked int) {
	log.Printf(
		"timer metrics: active=%d dedupe=%d activity=%d ends=%d fired=%d publish_errors=%d dropped=%d",
		activeTimers,
		dedupeTracked,
		s.metrics.activityEvents.Load(),
		s.metrics.endEvents.Load(),
		s.metrics.firedTimeouts.Load(),
		s.metrics.publishErrors.Load(),
		s.metrics.dropped.Load(),
	)
}
