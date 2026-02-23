package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/crimson-sun/lumber/internal/connector"
	"github.com/crimson-sun/lumber/internal/engine"
	"github.com/crimson-sun/lumber/internal/engine/dedup"
	"github.com/crimson-sun/lumber/internal/model"
	"github.com/crimson-sun/lumber/internal/output"
)

// Pipeline connects a connector, engine, and output into a processing pipeline.
type Pipeline struct {
	connector connector.Connector
	engine    *engine.Engine
	output    output.Output
	dedup     *dedup.Deduplicator
	window    time.Duration
}

// Option configures a Pipeline.
type Option func(*Pipeline)

// WithDedup enables event deduplication with the given Deduplicator and window.
func WithDedup(d *dedup.Deduplicator, window time.Duration) Option {
	return func(p *Pipeline) {
		p.dedup = d
		p.window = window
	}
}

// New creates a Pipeline from the given components.
func New(conn connector.Connector, eng *engine.Engine, out output.Output, opts ...Option) *Pipeline {
	p := &Pipeline{
		connector: conn,
		engine:    eng,
		output:    out,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Stream starts the pipeline in streaming mode, processing logs as they arrive.
// Blocks until the context is cancelled or an error occurs.
func (p *Pipeline) Stream(ctx context.Context, cfg connector.ConnectorConfig) error {
	ch, err := p.connector.Stream(ctx, cfg)
	if err != nil {
		return fmt.Errorf("pipeline stream: %w", err)
	}

	if p.dedup != nil {
		return p.streamWithDedup(ctx, ch)
	}
	return p.streamDirect(ctx, ch)
}

// streamDirect writes events directly without dedup.
func (p *Pipeline) streamDirect(ctx context.Context, ch <-chan model.RawLog) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case raw, ok := <-ch:
			if !ok {
				return nil
			}
			event, err := p.engine.Process(raw)
			if err != nil {
				return fmt.Errorf("pipeline process: %w", err)
			}
			if err := p.output.Write(ctx, event); err != nil {
				return fmt.Errorf("pipeline output: %w", err)
			}
		}
	}
}

// streamWithDedup buffers events and flushes deduplicated batches on a timer.
func (p *Pipeline) streamWithDedup(ctx context.Context, ch <-chan model.RawLog) error {
	buf := newStreamBuffer(p.dedup, p.output, p.window)

	for {
		select {
		case <-ctx.Done():
			// Flush remaining events on shutdown.
			if err := buf.flush(ctx); err != nil {
				return fmt.Errorf("pipeline flush on cancel: %w", err)
			}
			return ctx.Err()
		case raw, ok := <-ch:
			if !ok {
				// Channel closed — flush remaining.
				return buf.flush(ctx)
			}
			event, err := p.engine.Process(raw)
			if err != nil {
				return fmt.Errorf("pipeline process: %w", err)
			}
			buf.add(event)
		case <-buf.flushCh():
			if err := buf.flush(ctx); err != nil {
				return fmt.Errorf("pipeline flush: %w", err)
			}
		}
	}
}

// Query runs the pipeline in one-shot query mode.
func (p *Pipeline) Query(ctx context.Context, cfg connector.ConnectorConfig, params connector.QueryParams) error {
	raws, err := p.connector.Query(ctx, cfg, params)
	if err != nil {
		return fmt.Errorf("pipeline query: %w", err)
	}

	events, err := p.engine.ProcessBatch(raws)
	if err != nil {
		return fmt.Errorf("pipeline process batch: %w", err)
	}

	if p.dedup != nil {
		events = p.dedup.DeduplicateBatch(events)
	}

	for _, event := range events {
		if err := p.output.Write(ctx, event); err != nil {
			return fmt.Errorf("pipeline output: %w", err)
		}
	}
	return nil
}

// Close shuts down the output.
func (p *Pipeline) Close() error {
	return p.output.Close()
}
