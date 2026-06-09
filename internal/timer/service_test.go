package timer

import (
	"container/heap"
	"testing"
	"time"
)

func newHeapWith(entries ...*entry) *entryHeap {
	h := &entryHeap{}
	heap.Init(h)
	for _, e := range entries {
		heap.Push(h, e)
	}
	return h
}

func TestSafeCommitOffsets_NoPending(t *testing.T) {
	pk := partitionKey{topic: "act", partition: 0}
	maxSeen := map[partitionKey]int64{pk: 9}

	out := safeCommitOffsets(maxSeen, newHeapWith())

	if got := out[pk]; got != 10 {
		t.Fatalf("expected commit at 10 (maxSeen+1), got %d", got)
	}
}

func TestSafeCommitOffsets_GatesBehindOldestPending(t *testing.T) {
	pk := partitionKey{topic: "act", partition: 0}
	maxSeen := map[partitionKey]int64{pk: 20}

	// Two pending timers on the same partition at offsets 5 and 12.
	timers := newHeapWith(
		&entry{conversationID: "a", fireAt: time.Now().Add(time.Hour), ref: recordRef{topic: "act", partition: 0, offset: 12}},
		&entry{conversationID: "b", fireAt: time.Now().Add(time.Hour), ref: recordRef{topic: "act", partition: 0, offset: 5}},
	)

	out := safeCommitOffsets(maxSeen, timers)

	// Must not advance past the oldest pending record (offset 5).
	if got := out[pk]; got != 5 {
		t.Fatalf("expected commit gated at 5, got %d", got)
	}
}

func TestSafeCommitOffsets_DifferentPartitionsIndependent(t *testing.T) {
	act := partitionKey{topic: "act", partition: 0}
	end := partitionKey{topic: "end", partition: 0}
	maxSeen := map[partitionKey]int64{act: 20, end: 7}

	// Pending only on the activity partition.
	timers := newHeapWith(
		&entry{conversationID: "a", fireAt: time.Now().Add(time.Hour), ref: recordRef{topic: "act", partition: 0, offset: 8}},
	)

	out := safeCommitOffsets(maxSeen, timers)

	if got := out[act]; got != 8 {
		t.Fatalf("activity partition should gate at 8, got %d", got)
	}
	if got := out[end]; got != 8 {
		t.Fatalf("end partition should commit fully at 8 (maxSeen+1), got %d", got)
	}
}

func TestUpsertTimer_RefreshesDeadlineAndOffset(t *testing.T) {
	entries := make(map[string]*entry)
	timers := newHeapWith()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	upsertTimer(entries, timers, activityEvent{
		AgentID: "agent", ConversationID: "c1", OccurredAt: base,
	}, recordRef{topic: "act", partition: 0, offset: 1}, time.Minute)

	// Second activity for the same conversation refreshes fireAt and offset.
	upsertTimer(entries, timers, activityEvent{
		AgentID: "agent", ConversationID: "c1", OccurredAt: base.Add(30 * time.Second),
	}, recordRef{topic: "act", partition: 0, offset: 4}, time.Minute)

	if timers.Len() != 1 {
		t.Fatalf("expected single entry after upsert, got %d", timers.Len())
	}
	e := entries["c1"]
	if !e.fireAt.Equal(base.Add(30*time.Second + time.Minute)) {
		t.Fatalf("fireAt not refreshed: %v", e.fireAt)
	}
	if e.ref.offset != 4 {
		t.Fatalf("offset not refreshed, got %d", e.ref.offset)
	}
}

func TestRemoveTimer(t *testing.T) {
	entries := make(map[string]*entry)
	timers := newHeapWith()
	upsertTimer(entries, timers, activityEvent{
		AgentID: "agent", ConversationID: "c1", OccurredAt: time.Now(),
	}, recordRef{topic: "act", partition: 0, offset: 1}, time.Minute)

	removeTimer(entries, timers, "c1")
	if timers.Len() != 0 || len(entries) != 0 {
		t.Fatalf("entry not removed: timers=%d entries=%d", timers.Len(), len(entries))
	}

	// Removing an unknown id is a no-op.
	removeTimer(entries, timers, "missing")
}

func TestDedupe_IsEndedAndPrune(t *testing.T) {
	ended := map[string]time.Time{
		"fresh": time.Now(),
		"stale": time.Now().Add(-time.Hour),
	}

	if !isEnded(ended, "fresh", time.Minute) {
		t.Fatal("fresh id should be considered ended")
	}
	if isEnded(ended, "stale", time.Minute) {
		t.Fatal("stale id should have expired")
	}
	// isEnded should have deleted the stale entry lazily.
	if _, ok := ended["stale"]; ok {
		t.Fatal("stale entry should be pruned by isEnded")
	}

	ended["old"] = time.Now().Add(-time.Hour)
	pruneEnded(ended, time.Minute)
	if _, ok := ended["old"]; ok {
		t.Fatal("pruneEnded should drop expired entries")
	}
}

func TestEntryHeap_OrdersByFireAt(t *testing.T) {
	now := time.Now()
	timers := newHeapWith(
		&entry{conversationID: "late", fireAt: now.Add(2 * time.Minute)},
		&entry{conversationID: "soon", fireAt: now.Add(1 * time.Minute)},
		&entry{conversationID: "latest", fireAt: now.Add(3 * time.Minute)},
	)
	if (*timers)[0].conversationID != "soon" {
		t.Fatalf("expected earliest fireAt at root, got %s", (*timers)[0].conversationID)
	}
}
