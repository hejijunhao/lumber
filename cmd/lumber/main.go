package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kaminocorp/lumber/internal/cli"
	"github.com/kaminocorp/lumber/internal/config"
	"github.com/kaminocorp/lumber/internal/connector"
	"github.com/kaminocorp/lumber/internal/engine"
	"github.com/kaminocorp/lumber/internal/engine/classifier"
	"github.com/kaminocorp/lumber/internal/engine/compactor"
	"github.com/kaminocorp/lumber/internal/engine/dedup"
	"github.com/kaminocorp/lumber/internal/engine/embedder"
	"github.com/kaminocorp/lumber/internal/engine/taxonomy"
	"github.com/kaminocorp/lumber/internal/logging"
	"github.com/kaminocorp/lumber/internal/output"
	"github.com/kaminocorp/lumber/internal/output/async"
	"github.com/kaminocorp/lumber/internal/output/file"
	"github.com/kaminocorp/lumber/internal/output/multi"
	"github.com/kaminocorp/lumber/internal/output/stdout"
	"github.com/kaminocorp/lumber/internal/output/webhook"
	"github.com/kaminocorp/lumber/internal/pipeline"

	// Register connector implementations.
	_ "github.com/kaminocorp/lumber/internal/connector/file"
	_ "github.com/kaminocorp/lumber/internal/connector/flyio"
	_ "github.com/kaminocorp/lumber/internal/connector/stdin"
	_ "github.com/kaminocorp/lumber/internal/connector/supabase"
	_ "github.com/kaminocorp/lumber/internal/connector/vercel"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.LoadWithFlags()

	if cfg.ShowVersion {
		fmt.Printf("lumber %s\n", config.Version)
		return nil
	}

	// Print startup banner to stderr (doesn't mix with NDJSON on stdout).
	fmt.Fprintf(os.Stderr, "\n  lumber %s\n\n", config.Version)

	// Initialize logging early so wizard and model checks use the configured logger.
	logging.Init(cfg.Output.Format == "stdout", logging.ParseLevel(cfg.LogLevel))

	// Wizard / auto-detect logic: runs when no connector is configured.
	if cfg.Connector.Provider == "" {
		if isTerminal(os.Stdin) {
			// TTY — run interactive wizard.
			var err error
			cfg, err = cli.RunWizard(cfg)
			if err != nil {
				return fmt.Errorf("wizard: %w", err)
			}
		} else {
			// Piped input — auto-detect stdin connector.
			cfg.Connector.Provider = "stdin"
			cfg.Mode = "stream"
		}
	}

	// Model readiness check (non-wizard path).
	if !cli.ModelsReady(cfg) {
		if isTerminal(os.Stdin) {
			fmt.Fprintf(os.Stderr, "Model files not found. Run 'lumber' with no flags to launch the setup wizard,\n")
			fmt.Fprintf(os.Stderr, "or download manually: make download-model\n")
		} else {
			fmt.Fprintf(os.Stderr, "Model files not found at configured paths.\n")
			fmt.Fprintf(os.Stderr, "Download with: make download-model && make download-ort\n")
			fmt.Fprintf(os.Stderr, "Or set LUMBER_MODEL_PATH, LUMBER_VOCAB_PATH, LUMBER_PROJECTION_PATH\n")
		}
		return fmt.Errorf("model files not found")
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Initialize embedder.
	emb, err := embedder.New(cfg.Engine.ModelPath, cfg.Engine.VocabPath, cfg.Engine.ProjectionPath)
	if err != nil {
		return fmt.Errorf("creating embedder: %w", err)
	}
	defer emb.Close()
	slog.Info("embedder loaded", "model", cfg.Engine.ModelPath, "dim", emb.EmbedDim())

	// Initialize taxonomy with default labels.
	t0 := time.Now()
	tax, err := taxonomy.New(taxonomy.DefaultRoots(), emb)
	if err != nil {
		return fmt.Errorf("creating taxonomy: %w", err)
	}
	slog.Info("taxonomy pre-embedded", "labels", len(tax.Labels()), "duration", time.Since(t0).Round(time.Millisecond))

	// Initialize classifier and compactor.
	verbosity := parseVerbosity(cfg.Engine.Verbosity)
	cls := classifier.New(cfg.Engine.ConfidenceThreshold)
	cmp := compactor.New(verbosity)

	// Initialize engine.
	eng := engine.New(emb, tax, cls, cmp)

	// Initialize output(s).
	var outputs []output.Output
	outputs = append(outputs, stdout.New(verbosity, cfg.Output.Pretty))

	if cfg.Output.FilePath != "" {
		var fileOpts []file.Option
		if cfg.Output.FileMaxSize > 0 {
			fileOpts = append(fileOpts, file.WithMaxSize(cfg.Output.FileMaxSize))
		}
		f, err := file.New(cfg.Output.FilePath, verbosity, fileOpts...)
		if err != nil {
			return fmt.Errorf("creating file output: %w", err)
		}
		outputs = append(outputs, async.New(f))
		slog.Info("file output enabled", "path", cfg.Output.FilePath)
	}

	if cfg.Output.WebhookURL != "" {
		var whOpts []webhook.Option
		if cfg.Output.WebhookHeaders != nil {
			whOpts = append(whOpts, webhook.WithHeaders(cfg.Output.WebhookHeaders))
		}
		wh := webhook.New(cfg.Output.WebhookURL, whOpts...)
		outputs = append(outputs, async.New(wh, async.WithDropOnFull()))
		slog.Info("webhook output enabled", "url", redactURL(cfg.Output.WebhookURL))
	}

	out := multi.New(outputs...)

	// Resolve connector.
	ctor, err := connector.Get(cfg.Connector.Provider)
	if err != nil {
		return fmt.Errorf("getting connector: %w", err)
	}
	conn := ctor()

	// Build pipeline with optional dedup.
	var pipeOpts []pipeline.Option
	if cfg.Engine.DedupWindow > 0 {
		d := dedup.New(dedup.Config{Window: cfg.Engine.DedupWindow})
		pipeOpts = append(pipeOpts, pipeline.WithDedup(d, cfg.Engine.DedupWindow))
		slog.Info("dedup enabled", "window", cfg.Engine.DedupWindow)
	}
	if cfg.Engine.MaxBufferSize > 0 {
		pipeOpts = append(pipeOpts, pipeline.WithMaxBufferSize(cfg.Engine.MaxBufferSize))
	}
	p := pipeline.New(conn, eng, out, pipeOpts...)
	defer p.Close()

	// Set up graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 2) // buffer 2 to catch second signal
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig, "timeout", cfg.ShutdownTimeout)
		cancel()

		// Shutdown timer — force exit if drain exceeds timeout.
		timer := time.NewTimer(cfg.ShutdownTimeout)
		defer timer.Stop()

		select {
		case sig := <-sigCh:
			slog.Warn("second signal, forcing exit", "signal", sig)
			os.Exit(1)
		case <-timer.C:
			slog.Error("shutdown timeout exceeded, forcing exit", "timeout", cfg.ShutdownTimeout)
			os.Exit(1)
		}
	}()

	// Start pipeline.
	connCfg := connector.ConnectorConfig{
		Provider: cfg.Connector.Provider,
		APIKey:   cfg.Connector.APIKey,
		Endpoint: cfg.Connector.Endpoint,
		Extra:    cfg.Connector.Extra,
	}

	switch cfg.Mode {
	case "query":
		slog.Info("starting query", "connector", cfg.Connector.Provider,
			"from", cfg.QueryFrom, "to", cfg.QueryTo, "limit", cfg.QueryLimit)
		params := connector.QueryParams{
			Start: cfg.QueryFrom,
			End:   cfg.QueryTo,
			Limit: cfg.QueryLimit,
		}
		if err := p.Query(ctx, connCfg, params); err != nil {
			return fmt.Errorf("query failed: %w", err)
		}
	default: // "stream"
		slog.Info("starting stream", "connector", cfg.Connector.Provider)
		if err := p.Stream(ctx, connCfg); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("pipeline error: %w", err)
		}
	}

	return nil
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

// isTerminal reports whether f is connected to a terminal (TTY).
func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// redactURL removes query parameters from a URL for safe logging.
// Returns the original string if parsing fails.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.RawQuery != "" {
		u.RawQuery = "REDACTED"
	}
	return u.String()
}
