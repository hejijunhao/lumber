package async

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kaminocorp/lumber/internal/model"
	"github.com/kaminocorp/lumber/internal/output"
)

const (
	defaultBufferSize   = 1024
	defaultDrainTimeout = 5 * time.Second
)

// Option configures an Async wrapper.
type Option func(*Async)

// WithBufferSize sets the channel buffer capacity. Default: 1024.
func WithBufferSize(n int) Option {
	return func(a *Async) { a.bufSize = n }
}

// WithOnError sets the callback invoked when the inner output's Write fails.
// Default: logs a warning via slog.
func WithOnError(f func(error)) Option {
	return func(a *Async) { a.errFunc = f }
}

// WithDropOnFull makes Write return immediately (dropping the event) when the
// buffer is full, instead of blocking. Use for outputs where lossiness is
// acceptable (e.g., a non-critical webhook).
func WithDropOnFull() Option {
	return func(a *Async) { a.dropOnFull = true }
}

// Async decouples event production from consumption via a buffered channel.
// The pipeline writes into the channel; a background goroutine drains it
// to the wrapped output. Errors from the inner output are passed to errFunc
// rather than propagated to the caller.
type Async struct {
	inner      output.Output
	ch         chan model.CanonicalEvent
	done       chan struct{}       // closed when drain goroutine exits
	cancel     context.CancelFunc // cancels the drain context
	errFunc    func(error)
	bufSize    int
	dropOnFull bool
	closeOnce  sync.Once

	// mu protects closed flag to prevent send-on-closed-channel panics.
	mu     sync.RWMutex
	closed bool
}

// New wraps an output.Output in an async channel-based writer.
// The background drain goroutine starts immediately.
func New(inner output.Output, opts ...Option) *Async {
	a := &Async{
		inner:   inner,
		bufSize: defaultBufferSize,
		errFunc: func(err error) { slog.Warn("async output write error", "error", err) },
	}
	for _, opt := range opts {
		opt(a)
	}
	a.ch = make(chan model.CanonicalEvent, a.bufSize)
	a.done = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	go a.drain(ctx)
	return a
}

// Write sends the event into the channel. By default, blocks if the channel
// is full (backpressure). With WithDropOnFull, returns nil immediately and
// the event is lost. Returns an error if the wrapper has been closed.
func (a *Async) Write(_ context.Context, event model.CanonicalEvent) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return nil // silently discard after close
	}

	if a.dropOnFull {
		select {
		case a.ch <- event:
		default:
			slog.Warn("async output buffer full, dropping event",
				"type", event.Type, "category", event.Category)
		}
		return nil
	}
	a.ch <- event
	return nil
}

// Close marks the wrapper as closed, closes the channel, waits for the drain
// goroutine to finish (with a timeout), then closes the inner output.
func (a *Async) Close() error {
	var err error
	a.closeOnce.Do(func() {
		// Prevent new writes, then close the channel.
		a.mu.Lock()
		a.closed = true
		close(a.ch)
		a.mu.Unlock()

		// Wait for drain goroutine to finish.
		select {
		case <-a.done:
			// Drain goroutine finished normally.
		case <-time.After(defaultDrainTimeout):
			// Cancel in-flight inner.Write calls so drain can exit.
			a.cancel()
			remaining := len(a.ch)
			if remaining > 0 {
				slog.Warn("async output drain timed out, events lost", "dropped", remaining)
			} else {
				slog.Warn("async output drain timed out")
			}
			// Wait for drain goroutine to actually exit before closing inner.
			<-a.done
		}
		err = a.inner.Close()
	})
	return err
}

// drain reads events from the channel and writes them to the inner output.
// It exits when the channel is closed and fully drained.
func (a *Async) drain(ctx context.Context) {
	defer close(a.done)
	for event := range a.ch {
		if err := a.inner.Write(ctx, event); err != nil {
			a.errFunc(err)
		}
	}
}
