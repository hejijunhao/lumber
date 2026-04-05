package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/kaminocorp/lumber/internal/model"
)

const (
	defaultBatchSize     = 50
	defaultFlushInterval = 5 * time.Second
	defaultTimeout       = 10 * time.Second
	maxRetries           = 3
)

// Option configures a webhook Output.
type Option func(*Output)

// WithHeaders sets custom HTTP headers sent with every POST.
func WithHeaders(h map[string]string) Option {
	return func(o *Output) { o.headers = h }
}

// WithBatchSize sets the number of events accumulated before a flush. Default: 50.
func WithBatchSize(n int) Option {
	return func(o *Output) { o.batchSize = n }
}

// WithFlushInterval sets the maximum time between flushes. Default: 5s.
func WithFlushInterval(d time.Duration) Option {
	return func(o *Output) { o.flushInterval = d }
}

// WithTimeout sets the HTTP client timeout. Default: 10s.
func WithTimeout(d time.Duration) Option {
	return func(o *Output) { o.client.Timeout = d }
}

// WithOnError sets a callback invoked when a timer-triggered flush fails.
// Default: logs a warning via slog.
func WithOnError(f func(error)) Option {
	return func(o *Output) { o.errFunc = f }
}

// Output POSTs batched canonical events to an HTTP endpoint as a JSON array.
// Events accumulate in an internal buffer and are flushed when batchSize is
// reached or flushInterval elapses. Retries on 5xx with exponential backoff.
type Output struct {
	client        *http.Client
	url           string
	headers       map[string]string
	batchSize     int
	flushInterval time.Duration
	errFunc       func(error)
	mu            sync.Mutex
	pending       []model.CanonicalEvent
	timer         *time.Timer
	wg            sync.WaitGroup // tracks in-flight POST goroutines
}

// New creates a webhook output targeting the given URL.
func New(url string, opts ...Option) *Output {
	o := &Output{
		client:        &http.Client{Timeout: defaultTimeout},
		url:           url,
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
		errFunc:       func(err error) { slog.Warn("webhook flush error", "error", err) },
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// Write appends an event to the batch. When batchSize is reached, the batch
// is flushed immediately. A timer is started on the first event to ensure
// the batch flushes even if batchSize is never reached.
func (o *Output) Write(_ context.Context, event model.CanonicalEvent) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.pending = append(o.pending, event)

	if len(o.pending) >= o.batchSize {
		return o.flushLocked()
	}

	// Start timer on first event in a new batch.
	if len(o.pending) == 1 {
		o.timer = time.AfterFunc(o.flushInterval, func() {
			o.mu.Lock()
			defer o.mu.Unlock()
			if err := o.flushLocked(); err != nil {
				o.errFunc(err)
			}
		})
	}
	return nil
}

// Close flushes any remaining events, stops the timer, and waits for
// in-flight POST requests to complete.
func (o *Output) Close() error {
	o.mu.Lock()
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}
	var err error
	if len(o.pending) > 0 {
		err = o.flushLocked()
	}
	o.mu.Unlock()

	// Wait for all in-flight POST requests to complete.
	o.wg.Wait()
	return err
}

// flushLocked takes the pending batch under the lock, releases, and sends
// via HTTP POST in the background. Caller must hold o.mu.
func (o *Output) flushLocked() error {
	if len(o.pending) == 0 {
		return nil
	}
	if o.timer != nil {
		o.timer.Stop()
		o.timer = nil
	}

	batch := o.pending
	o.pending = make([]model.CanonicalEvent, 0, o.batchSize)

	body, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	// Send the batch in a background goroutine so we don't hold the mutex
	// during HTTP calls (which may include retry sleeps).
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		if err := o.postWithRetry(body); err != nil {
			slog.Warn("webhook batch lost", "error", err, "events", len(batch))
			o.errFunc(err)
		}
	}()

	return nil
}

// postWithRetry sends the body via HTTP POST with retry on 5xx and network errors.
func (o *Output) postWithRetry(body []byte) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<(attempt-1)) * time.Second)
		}

		req, err := http.NewRequest(http.MethodPost, o.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("webhook: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range o.headers {
			req.Header.Set(k, v)
		}

		resp, err := o.client.Do(req)
		if err != nil {
			// Retry network errors.
			lastErr = fmt.Errorf("webhook: %w", err)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		lastErr = fmt.Errorf("webhook: HTTP %d", resp.StatusCode)

		// Only retry on 5xx server errors.
		if resp.StatusCode < 500 {
			return lastErr
		}
	}
	return lastErr
}
