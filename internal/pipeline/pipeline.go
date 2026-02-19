package pipeline

import (
	"context"
	"fmt"

	"github.com/crimson-sun/lumber/internal/connector"
	"github.com/crimson-sun/lumber/internal/engine"
	"github.com/crimson-sun/lumber/internal/output"
)

// Pipeline connects a connector, engine, and output into a processing pipeline.
type Pipeline struct {
	connector connector.Connector
	engine    *engine.Engine
	output    output.Output
}

// New creates a Pipeline from the given components.
func New(conn connector.Connector, eng *engine.Engine, out output.Output) *Pipeline {
	return &Pipeline{
		connector: conn,
		engine:    eng,
		output:    out,
	}
}

// Stream starts the pipeline in streaming mode, processing logs as they arrive.
// Blocks until the context is cancelled or an error occurs.
func (p *Pipeline) Stream(ctx context.Context, cfg connector.ConnectorConfig) error {
	ch, err := p.connector.Stream(ctx, cfg)
	if err != nil {
		return fmt.Errorf("pipeline stream: %w", err)
	}

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
