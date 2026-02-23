package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/crimson-sun/lumber/internal/engine/dedup"
	"github.com/crimson-sun/lumber/internal/model"
	"github.com/crimson-sun/lumber/internal/output"
)

// streamBuffer accumulates events and flushes deduplicated batches on a timer.
type streamBuffer struct {
	dedup  *dedup.Deduplicator
	out    output.Output
	window time.Duration

	mu      sync.Mutex
	pending []model.CanonicalEvent
	timer   *time.Timer
}

func newStreamBuffer(d *dedup.Deduplicator, out output.Output, window time.Duration) *streamBuffer {
	return &streamBuffer{
		dedup:  d,
		out:    out,
		window: window,
	}
}

// add appends an event to the buffer. If this is the first event, starts the flush timer.
func (b *streamBuffer) add(event model.CanonicalEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.pending = append(b.pending, event)
	if len(b.pending) == 1 {
		// First event — start timer.
		b.timer = time.NewTimer(b.window)
	}
}

// flushCh returns the timer's channel, or nil if no timer is active.
func (b *streamBuffer) flushCh() <-chan time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.timer == nil {
		return nil
	}
	return b.timer.C
}

// flush deduplicates and writes all pending events.
func (b *streamBuffer) flush(ctx context.Context) error {
	b.mu.Lock()
	events := b.pending
	b.pending = nil
	b.timer = nil
	b.mu.Unlock()

	if len(events) == 0 {
		return nil
	}

	deduped := b.dedup.DeduplicateBatch(events)
	for _, e := range deduped {
		if err := b.out.Write(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
