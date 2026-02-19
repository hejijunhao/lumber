package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/crimson-sun/lumber/internal/config"
	"github.com/crimson-sun/lumber/internal/connector"
	"github.com/crimson-sun/lumber/internal/engine"
	"github.com/crimson-sun/lumber/internal/engine/classifier"
	"github.com/crimson-sun/lumber/internal/engine/compactor"
	"github.com/crimson-sun/lumber/internal/engine/embedder"
	"github.com/crimson-sun/lumber/internal/engine/taxonomy"
	"github.com/crimson-sun/lumber/internal/output/stdout"
	"github.com/crimson-sun/lumber/internal/pipeline"

	// Register connector implementations.
	_ "github.com/crimson-sun/lumber/internal/connector/vercel"
)

func main() {
	cfg := config.Load()

	// Initialize embedder.
	emb, err := embedder.New(cfg.Engine.ModelPath)
	if err != nil {
		log.Fatalf("failed to create embedder: %v", err)
	}

	// Initialize taxonomy with default labels.
	tax, err := taxonomy.New(taxonomy.DefaultRoots(), emb)
	if err != nil {
		log.Fatalf("failed to create taxonomy: %v", err)
	}

	// Initialize classifier and compactor.
	cls := classifier.New(cfg.Engine.ConfidenceThreshold)
	cmp := compactor.New(parseVerbosity(cfg.Engine.Verbosity))

	// Initialize engine.
	eng := engine.New(emb, tax, cls, cmp)

	// Initialize output.
	out := stdout.New()

	// Resolve connector.
	ctor, err := connector.Get(cfg.Connector.Provider)
	if err != nil {
		log.Fatalf("failed to get connector: %v", err)
	}
	conn := ctor()

	// Build pipeline.
	p := pipeline.New(conn, eng, out)
	defer p.Close()

	// Set up graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nreceived %v, shutting down...\n", sig)
		cancel()
	}()

	// Start streaming.
	connCfg := connector.ConnectorConfig{
		Provider: cfg.Connector.Provider,
		APIKey:   cfg.Connector.APIKey,
		Endpoint: cfg.Connector.Endpoint,
	}

	fmt.Fprintf(os.Stderr, "lumber: starting with connector=%s\n", cfg.Connector.Provider)
	if err := p.Stream(ctx, connCfg); err != nil && err != context.Canceled {
		log.Fatalf("pipeline error: %v", err)
	}
}

func parseVerbosity(s string) compactor.Verbosity {
	switch s {
	case "minimal":
		return compactor.Minimal
	case "full":
		return compactor.Full
	default:
		return compactor.Standard
	}
}
