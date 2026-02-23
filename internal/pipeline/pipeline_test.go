package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/crimson-sun/lumber/internal/engine/dedup"
	"github.com/crimson-sun/lumber/internal/model"
)

// --- mocks ---

type mockOutput struct {
	mu     sync.Mutex
	events []model.CanonicalEvent
}

func (m *mockOutput) Write(_ context.Context, e model.CanonicalEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, e)
	return nil
}

func (m *mockOutput) Close() error { return nil }

func (m *mockOutput) Events() []model.CanonicalEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]model.CanonicalEvent, len(m.events))
	copy(cp, m.events)
	return cp
}

// --- streamBuffer tests ---

func TestStreamBufferFlush(t *testing.T) {
	out := &mockOutput{}
	d := dedup.New(dedup.Config{Window: time.Second})
	buf := newStreamBuffer(d, out, 100*time.Millisecond)

	t0 := time.Now()
	// Add 10 identical events.
	for i := 0; i < 10; i++ {
		buf.add(model.CanonicalEvent{
			Type:      "ERROR",
			Category:  "timeout",
			Severity:  "error",
			Timestamp: t0.Add(time.Duration(i) * time.Millisecond),
			Summary:   "timeout",
		})
	}

	// Wait for timer to fire.
	select {
	case <-buf.flushCh():
	case <-time.After(time.Second):
		t.Fatal("flush timer didn't fire")
	}

	if err := buf.flush(context.Background()); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	events := out.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 deduplicated event, got %d", len(events))
	}
	if events[0].Count != 10 {
		t.Fatalf("expected Count=10, got %d", events[0].Count)
	}
}

func TestStreamBufferContextCancel(t *testing.T) {
	out := &mockOutput{}
	d := dedup.New(dedup.Config{Window: 10 * time.Second})
	buf := newStreamBuffer(d, out, 10*time.Second) // Long window — won't fire.

	t0 := time.Now()
	buf.add(model.CanonicalEvent{
		Type:      "ERROR",
		Category:  "timeout",
		Severity:  "error",
		Timestamp: t0,
		Summary:   "timeout",
	})
	buf.add(model.CanonicalEvent{
		Type:      "ERROR",
		Category:  "timeout",
		Severity:  "error",
		Timestamp: t0.Add(time.Second),
		Summary:   "timeout",
	})

	// Flush immediately (simulating context cancel).
	if err := buf.flush(context.Background()); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	events := out.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 deduplicated event on cancel flush, got %d", len(events))
	}
	if events[0].Count != 2 {
		t.Fatalf("expected Count=2, got %d", events[0].Count)
	}
}

func TestPipelineWithoutDedup(t *testing.T) {
	// Verify that a pipeline without dedup passes events directly.
	out := &mockOutput{}
	d := dedup.New(dedup.Config{Window: time.Second})
	buf := newStreamBuffer(d, out, 50*time.Millisecond)

	// Add 3 distinct events.
	t0 := time.Now()
	buf.add(model.CanonicalEvent{Type: "ERROR", Category: "timeout", Timestamp: t0, Summary: "a"})
	buf.add(model.CanonicalEvent{Type: "REQUEST", Category: "success", Timestamp: t0, Summary: "b"})
	buf.add(model.CanonicalEvent{Type: "DEPLOY", Category: "build_succeeded", Timestamp: t0, Summary: "c"})

	select {
	case <-buf.flushCh():
	case <-time.After(time.Second):
		t.Fatal("flush timer didn't fire")
	}

	if err := buf.flush(context.Background()); err != nil {
		t.Fatalf("flush error: %v", err)
	}

	events := out.Events()
	if len(events) != 3 {
		t.Fatalf("expected 3 distinct events, got %d", len(events))
	}
}
