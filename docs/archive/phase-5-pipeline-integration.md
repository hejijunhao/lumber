# Phase 5: Pipeline Integration & Resilience — Implementation Plan

## Goal

Make the full pipeline (connector → engine → output) run reliably against real log data with proper error handling, buffering, graceful shutdown, structured internal logging, CLI access to both stream and query modes, and config validation at startup. All new code covered by unit tests that run without ONNX model files. End-to-end integration tests (requiring ONNX) validate the full data path.

**Success criteria:**
- Structured internal logging via `log/slog` (no more mixed `fmt.Fprintf`/`log.Printf`/`log.Fatalf`)
- Config validation at startup — fail fast on missing files, invalid thresholds, bad verbosity
- Per-log error resilience — one bad log is skipped, not fatal; pipeline continues
- Bounded dedup buffer — force-flush at max size, no unbounded memory growth
- Graceful shutdown with configurable timeout and double-signal force exit
- CLI flags for mode (stream/query), connector, time range, verbosity, pretty, log level
- Query mode accessible from the CLI (currently pipeline supports it but main.go doesn't)
- End-to-end integration test: httptest → Vercel connector → real engine → mock output
- `go build ./cmd/lumber` compiles, all new tests pass, `./lumber -help` shows flags

---

## Current State

**Working:**
- `Pipeline.Stream()` — reads connector channel, processes via engine, writes to output
- `Pipeline.Query()` — fetches batch, processes via engine, writes to output
- `Pipeline.streamWithDedup()` — timer-based dedup buffer, flushes on cancel
- `streamBuffer` — mutex-protected pending slice, lazy timer start
- Signal handling — SIGINT/SIGTERM cancel context
- Config via env vars — `LUMBER_CONNECTOR`, `LUMBER_API_KEY`, `LUMBER_VERBOSITY`, etc.
- Three connectors (Vercel/Fly.io/Supabase) with `httptest`-based test suites
- All 104 tests passing, binary compiles

**Known issues:**
- `streamDirect()` calls `return fmt.Errorf(...)` on `engine.Process()` error — one bad log kills the pipeline
- `streamWithDedup()` has the same fatal error pattern
- `streamBuffer.pending` has no max size — unbounded memory growth during log storms
- Signal handler has no shutdown timeout — process hangs indefinitely if flush blocks
- No config validation — missing model files cause cryptic ONNX errors later
- Logging is inconsistent: `fmt.Fprintf(os.Stderr)` in main, `log.Printf` in connectors, `log.Fatalf` for fatal
- `main.go` always calls `p.Stream()` — query mode is unreachable from the CLI
- No CLI flags — env vars only, no `-help`

---

## Section 1: Structured Internal Logging

**What:** Replace the three inconsistent logging patterns with `log/slog` (stdlib since Go 1.21). Every subsequent section depends on having a structured logger.

### Tasks

1.1 **Create `internal/logging/logging.go`** (~30 lines)

Public API:

```go
// Init creates and sets the package-level default slog logger.
// When outputIsStdout is true, uses JSONHandler on stderr (avoids mixing with NDJSON output).
// Otherwise uses TextHandler on stderr for human readability.
func Init(outputIsStdout bool, level slog.Level)

// ParseLevel converts a string ("debug", "info", "warn", "error") to slog.Level.
// Unknown strings default to LevelInfo.
func ParseLevel(s string) slog.Level
```

Uses `slog.SetDefault()` — call sites use `slog.Info(...)`, `slog.Warn(...)` etc. directly. No logger passing. The handler is always attached to `os.Stderr`.

1.2 **Add `LogLevel` to `Config`** in `internal/config/config.go`.

In the top-level `Config` struct (not nested inside Engine/Output — it's a process-wide setting):

```go
type Config struct {
    Connector ConnectorConfig
    Engine    EngineConfig
    Output    OutputConfig
    LogLevel  string // "debug", "info", "warn", "error"
}
```

In `Load()`:

```go
LogLevel: getenv("LUMBER_LOG_LEVEL", "info"),
```

1.3 **Update `cmd/lumber/main.go`** — call `logging.Init()` right after `config.Load()`:

```go
cfg := config.Load()
logging.Init(cfg.Output.Format == "stdout", logging.ParseLevel(cfg.LogLevel))
```

Replace all 5 `fmt.Fprintf(os.Stderr, "lumber: ...")` calls with structured `slog.Info(...)`:

| Before | After |
|--------|-------|
| `fmt.Fprintf(os.Stderr, "lumber: embedder loaded model=%s dim=%d\n", ...)` | `slog.Info("embedder loaded", "model", cfg.Engine.ModelPath, "dim", emb.EmbedDim())` |
| `fmt.Fprintf(os.Stderr, "lumber: taxonomy pre-embedded %d labels in %s\n", ...)` | `slog.Info("taxonomy pre-embedded", "labels", len(tax.Labels()), "duration", time.Since(t0).Round(time.Millisecond))` |
| `fmt.Fprintf(os.Stderr, "lumber: dedup enabled window=%s\n", ...)` | `slog.Info("dedup enabled", "window", cfg.Engine.DedupWindow)` |
| `fmt.Fprintf(os.Stderr, "\nreceived %v, shutting down...\n", sig)` | `slog.Info("shutting down", "signal", sig)` |
| `fmt.Fprintf(os.Stderr, "lumber: starting with connector=%s\n", ...)` | `slog.Info("starting", "connector", cfg.Connector.Provider)` |

Keep `log.Fatalf` for pre-`logging.Init()` fatal errors (there are none — `config.Load()` can't fail). Post-init fatal errors become `slog.Error(...)` + `os.Exit(1)`.

1.4 **Update connectors** — replace `log.Printf(...)` with `slog.Warn(...)`:

- `internal/connector/vercel/vercel.go` line 182: `log.Printf("vercel connector: poll error: %v", err)` → `slog.Warn("poll error", "connector", "vercel", "error", err)`
- `internal/connector/flyio/flyio.go` line 177: same pattern → `slog.Warn("poll error", "connector", "flyio", "error", err)`
- `internal/connector/supabase/supabase.go` line 235: `log.Printf("supabase: buildSQL error for table %s: %v", ...)` → `slog.Warn("sql build error", "connector", "supabase", "table", table, "error", err)`
- `internal/connector/supabase/supabase.go` line 249: `log.Printf("supabase: poll error for table %s: %v", ...)` → `slog.Warn("poll error", "connector", "supabase", "table", table, "error", err)`

1.5 **Create `internal/logging/logging_test.go`** (3 tests)

### Files

| File | Action |
|------|--------|
| `internal/logging/logging.go` | New |
| `internal/logging/logging_test.go` | New |
| `internal/config/config.go` | Add `LogLevel` field to `Config`, env var |
| `cmd/lumber/main.go` | Import logging, call Init, replace all fprintf/printf |
| `internal/connector/vercel/vercel.go` | `log.Printf` → `slog.Warn` |
| `internal/connector/flyio/flyio.go` | `log.Printf` → `slog.Warn` |
| `internal/connector/supabase/supabase.go` | `log.Printf` → `slog.Warn` (×2) |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestParseLevel` | Each string maps to correct `slog.Level`; unknown defaults to Info |
| `TestInitJSON` | `Init(true, ...)` produces JSON-formatted output |
| `TestInitText` | `Init(false, ...)` produces text-formatted output |

### Verification

```
go test ./internal/logging/...
go build ./cmd/lumber
```

---

## Section 2: Config Validation

**What:** Add `Validate() error` to `Config`. Call in `main.go` immediately after `Load()`, before any component initialization.

### Tasks

2.1 **Add `Validate() error` method to `Config`** in `internal/config/config.go` (~40 lines).

Collects all errors into a slice, returns them all (not just the first):

```go
func (c Config) Validate() error {
    var errs []string

    // API key required when provider is set.
    if c.Connector.Provider != "" && c.Connector.APIKey == "" {
        errs = append(errs, "LUMBER_API_KEY is required when a connector is configured")
    }

    // Model files must exist on disk.
    for _, f := range []struct{ name, path string }{
        {"model", c.Engine.ModelPath},
        {"vocab", c.Engine.VocabPath},
        {"projection", c.Engine.ProjectionPath},
    } {
        if _, err := os.Stat(f.path); os.IsNotExist(err) {
            errs = append(errs, fmt.Sprintf("%s file not found: %s", f.name, f.path))
        }
    }

    // Confidence threshold in [0, 1].
    if c.Engine.ConfidenceThreshold < 0 || c.Engine.ConfidenceThreshold > 1 {
        errs = append(errs, fmt.Sprintf("confidence threshold must be 0-1, got %f", c.Engine.ConfidenceThreshold))
    }

    // Verbosity enum.
    switch c.Engine.Verbosity {
    case "minimal", "standard", "full":
    default:
        errs = append(errs, fmt.Sprintf("invalid verbosity %q (must be minimal|standard|full)", c.Engine.Verbosity))
    }

    // Dedup window non-negative.
    if c.Engine.DedupWindow < 0 {
        errs = append(errs, fmt.Sprintf("dedup window must be non-negative, got %s", c.Engine.DedupWindow))
    }

    // Mode enum (added in Section 6, validated here once Mode field exists).
    // Will be added in Section 6.

    if len(errs) > 0 {
        return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
    }
    return nil
}
```

Note: No import of `connector` package (avoids import cycle). The provider name check stays in `main.go` where `connector.Get()` already validates it with a clear error message.

2.2 **Call `Validate()` in `main.go`** after `Load()` and `logging.Init()`, before embedder creation:

```go
if err := cfg.Validate(); err != nil {
    slog.Error("invalid configuration", "error", err)
    os.Exit(1)
}
```

### Files

| File | Action |
|------|--------|
| `internal/config/config.go` | Add `Validate()` method, add `"fmt"` and `"strings"` imports |
| `internal/config/config_test.go` | Add 7 validation tests |
| `cmd/lumber/main.go` | Call `cfg.Validate()` after logging.Init() |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestValidate_ValidConfig` | Valid config with temp files → nil error |
| `TestValidate_BadConfidenceThreshold` | 1.5 → error containing "confidence" |
| `TestValidate_BadVerbosity` | `"verbose"` → error containing "verbosity" |
| `TestValidate_NegativeDedupWindow` | -1s → error |
| `TestValidate_MissingModelFile` | Non-existent path → error containing "model" |
| `TestValidate_MissingAPIKey` | Provider set, APIKey empty → error |
| `TestValidate_MultipleErrors` | Multiple bad fields → all errors listed in output |

### Verification

```
go test ./internal/config/...
go build ./cmd/lumber
```

---

## Section 3: Per-Log Error Handling

**What:** `engine.Process()` failures log a warning and skip the log. `output.Write()` failures remain fatal. This is the single most critical reliability fix.

### Tasks

3.1 **Define `Processor` interface** in `internal/pipeline/pipeline.go`:

```go
// Processor handles log classification and compaction.
type Processor interface {
    Process(raw model.RawLog) (model.CanonicalEvent, error)
    ProcessBatch(raws []model.RawLog) ([]model.CanonicalEvent, error)
}
```

Change `Pipeline.engine` field from `*engine.Engine` to `Processor`. Change `New()` parameter from `*engine.Engine` to `Processor`. The real `*engine.Engine` already satisfies this interface — `main.go` compiles without changes. This enables mock engines in tests for error injection.

3.2 **Add atomic skip counter** to `Pipeline`:

```go
import "sync/atomic"

type Pipeline struct {
    connector    connector.Connector
    engine       Processor
    output       output.Output
    dedup        *dedup.Deduplicator
    window       time.Duration
    skippedLogs  atomic.Int64
}
```

Report in `Close()`:

```go
func (p *Pipeline) Close() error {
    if skipped := p.skippedLogs.Load(); skipped > 0 {
        slog.Info("pipeline closing", "total_skipped_logs", skipped)
    }
    return p.output.Close()
}
```

3.3 **Update `streamDirect()`** — skip on Process error:

```go
func (p *Pipeline) streamDirect(ctx context.Context, ch <-chan model.RawLog) error {
    for {
        select {
        case <-ctx.Done():
            if skipped := p.skippedLogs.Load(); skipped > 0 {
                slog.Info("stream stopped", "skipped_logs", skipped)
            }
            return ctx.Err()
        case raw, ok := <-ch:
            if !ok {
                if skipped := p.skippedLogs.Load(); skipped > 0 {
                    slog.Info("stream ended", "skipped_logs", skipped)
                }
                return nil
            }
            event, err := p.engine.Process(raw)
            if err != nil {
                p.skippedLogs.Add(1)
                slog.Warn("skipping log", "error", err, "source", raw.Source)
                continue
            }
            if err := p.output.Write(ctx, event); err != nil {
                return fmt.Errorf("pipeline output: %w", err)
            }
        }
    }
}
```

3.4 **Update `streamWithDedup()`** — same skip pattern for Process errors. Output/flush errors remain fatal.

3.5 **Update `Query()`** — fallback on batch failure:

```go
func (p *Pipeline) Query(ctx context.Context, cfg connector.ConnectorConfig, params connector.QueryParams) error {
    raws, err := p.connector.Query(ctx, cfg, params)
    if err != nil {
        return fmt.Errorf("pipeline query: %w", err)
    }

    events, err := p.engine.ProcessBatch(raws)
    if err != nil {
        slog.Warn("batch processing failed, falling back to individual", "error", err, "count", len(raws))
        events = p.processIndividual(raws)
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
```

3.6 **Add `processIndividual` helper** — skip-and-continue per log:

```go
func (p *Pipeline) processIndividual(raws []model.RawLog) []model.CanonicalEvent {
    var events []model.CanonicalEvent
    for _, raw := range raws {
        event, err := p.engine.Process(raw)
        if err != nil {
            p.skippedLogs.Add(1)
            slog.Warn("skipping log in query", "error", err, "source", raw.Source)
            continue
        }
        events = append(events, event)
    }
    return events
}
```

### Files

| File | Action |
|------|--------|
| `internal/pipeline/pipeline.go` | Processor interface, skip logic, atomic counter, processIndividual helper |
| `internal/pipeline/pipeline_test.go` | Add error handling tests with mock processor |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestStreamDirect_SkipsBadLog` | Mock processor fails on specific input; pipeline continues; good logs in output; bad logs skipped |
| `TestStreamWithDedup_SkipsBadLog` | Same with dedup enabled |
| `TestQuery_BatchFallback` | Mock processor where ProcessBatch fails; verify fallback to individual processing |
| `TestSkipCounter` | Counter increments correctly; reported on Close |

Tests use a `mockProcessor` that returns an error when `raw.Raw` matches a specific string. All other inputs return a fixed `CanonicalEvent`. No ONNX required.

### Verification

```
go test ./internal/pipeline/...
go build ./cmd/lumber
```

---

## Section 4: Bounded Dedup Buffer

**What:** Add max size to `streamBuffer`. When buffer hits max, force an immediate flush. No events dropped.

### Tasks

4.1 **Add `maxSize` field to `streamBuffer`** in `internal/pipeline/buffer.go`:

```go
type streamBuffer struct {
    dedup   *dedup.Deduplicator
    out     output.Output
    window  time.Duration
    maxSize int // 0 means unlimited (backward compat)

    mu      sync.Mutex
    pending []model.CanonicalEvent
    timer   *time.Timer
}
```

Update constructor:

```go
func newStreamBuffer(d *dedup.Deduplicator, out output.Output, window time.Duration, maxSize int) *streamBuffer
```

4.2 **Modify `add()` to return a flush signal**:

```go
// add appends an event. Returns true if the buffer is full and needs flushing.
func (b *streamBuffer) add(event model.CanonicalEvent) bool {
    b.mu.Lock()
    defer b.mu.Unlock()

    b.pending = append(b.pending, event)
    if len(b.pending) == 1 {
        b.timer = time.NewTimer(b.window)
    }
    return b.maxSize > 0 && len(b.pending) >= b.maxSize
}
```

4.3 **Update `streamWithDedup()`** to handle early flush:

```go
if buf.add(event) {
    // Buffer full — force early flush.
    if err := buf.flush(ctx); err != nil {
        return fmt.Errorf("pipeline flush (buffer full): %w", err)
    }
}
```

4.4 **Add `MaxBufferSize` to `EngineConfig`** and `getenvInt` helper:

```go
MaxBufferSize int // max events buffered before force flush; 0 = unlimited
```

```go
MaxBufferSize: getenvInt("LUMBER_MAX_BUFFER_SIZE", 1000),
```

```go
func getenvInt(key string, fallback int) int {
    v := os.Getenv(key)
    if v == "" {
        return fallback
    }
    n, err := strconv.Atoi(v)
    if err != nil {
        return fallback
    }
    return n
}
```

4.5 **Add `WithMaxBufferSize(n int) Option`** to Pipeline:

```go
func WithMaxBufferSize(n int) Option {
    return func(p *Pipeline) {
        p.maxBufferSize = n
    }
}
```

Add `maxBufferSize int` field to `Pipeline`. Pass to `newStreamBuffer()` in `streamWithDedup()`.

4.6 **Update `cmd/lumber/main.go`** — pass `pipeline.WithMaxBufferSize(cfg.Engine.MaxBufferSize)`.

### Files

| File | Action |
|------|--------|
| `internal/pipeline/buffer.go` | Add maxSize field, bool return from add() |
| `internal/pipeline/pipeline.go` | Add maxBufferSize field + Option, handle early flush, wire to buffer |
| `internal/config/config.go` | Add MaxBufferSize to EngineConfig, getenvInt helper |
| `cmd/lumber/main.go` | Pass WithMaxBufferSize |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestStreamBuffer_MaxSizeFlush` | Add maxSize events; `add()` returns true on the last |
| `TestStreamBuffer_MaxSizeNoDataLoss` | Fill to max, flush, add more; verify all events accounted for |
| `TestStreamBuffer_UnlimitedBackcompat` | maxSize=0 allows unbounded growth (existing behavior) |
| `TestGetenvInt` | Int parsing helper with valid, invalid, and empty inputs |

### Verification

```
go test ./internal/pipeline/...
go test ./internal/config/...
```

---

## Section 5: Graceful Shutdown

**What:** Bounded shutdown timeout. Second signal forces immediate exit.

### Tasks

5.1 **Add `ShutdownTimeout` to `Config`**:

```go
type Config struct {
    Connector       ConnectorConfig
    Engine          EngineConfig
    Output          OutputConfig
    LogLevel        string
    ShutdownTimeout time.Duration
}
```

In `Load()`:

```go
ShutdownTimeout: getenvDuration("LUMBER_SHUTDOWN_TIMEOUT", 10*time.Second),
```

5.2 **Rewrite signal handling in `main.go`**:

```go
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
```

5.3 **Fix final flush context in `streamWithDedup()`**.

Currently passes the already-cancelled `ctx` to `buf.flush(ctx)`. If output.Write checks ctx, it fails immediately. Fix: use `context.Background()` for the final drain flush — the shutdown timer in main.go provides the hard bound.

```go
case <-ctx.Done():
    // ... log skipped count ...
    // Use background context so writes can complete during drain.
    if err := buf.flush(context.Background()); err != nil {
        return fmt.Errorf("pipeline flush on shutdown: %w", err)
    }
    return ctx.Err()
```

### Files

| File | Action |
|------|--------|
| `internal/config/config.go` | Add ShutdownTimeout to Config |
| `cmd/lumber/main.go` | Rewrite signal handler with timeout + double-signal |
| `internal/pipeline/pipeline.go` | Use `context.Background()` for final dedup flush |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestLoad_ShutdownTimeoutDefault` | Default is 10s |
| `TestLoad_ShutdownTimeoutEnv` | `LUMBER_SHUTDOWN_TIMEOUT=5s` parsed correctly |
| `TestStreamWithDedup_FlushOnCancel` | Extend existing test to verify buffer fully flushed on cancel |

Signal handling (double-signal, timeout) is difficult to unit test (OS process signals). Verified manually.

### Verification

```
go test ./internal/config/...
go test ./internal/pipeline/...
go build ./cmd/lumber
```

---

## Section 6: CLI Flags

**What:** Add CLI flags via stdlib `flag` package. Flags override env vars. Add query mode to `main.go`.

### Tasks

6.1 **Add Mode and query fields to `Config`**:

```go
type Config struct {
    Connector       ConnectorConfig
    Engine          EngineConfig
    Output          OutputConfig
    LogLevel        string
    ShutdownTimeout time.Duration
    Mode            string    // "stream" or "query"
    QueryFrom       time.Time // query start time (RFC3339)
    QueryTo         time.Time // query end time (RFC3339)
    QueryLimit      int       // max results; 0 = no limit
}
```

In `Load()`:

```go
Mode: getenv("LUMBER_MODE", "stream"),
```

6.2 **Add `LoadWithFlags() Config`** (~50 lines).

Preserves `Load()` for tests (no flag parsing). `LoadWithFlags` calls `Load()` first, then defines flags, calls `flag.Parse()`, and overrides only explicitly-set flags using `flag.Visit()`:

```go
func LoadWithFlags() Config {
    cfg := Load()

    mode := flag.String("mode", "", "Pipeline mode: stream or query")
    connFlag := flag.String("connector", "", "Connector: vercel, flyio, supabase")
    from := flag.String("from", "", "Query start time (RFC3339)")
    to := flag.String("to", "", "Query end time (RFC3339)")
    limit := flag.Int("limit", 0, "Query result limit")
    verbosity := flag.String("verbosity", "", "Verbosity: minimal, standard, full")
    pretty := flag.Bool("pretty", false, "Pretty-print JSON output")
    logLevel := flag.String("log-level", "", "Log level: debug, info, warn, error")

    flag.Parse()

    // Override only explicitly-set flags.
    flag.Visit(func(f *flag.Flag) {
        switch f.Name {
        case "mode":
            cfg.Mode = *mode
        case "connector":
            cfg.Connector.Provider = *connFlag
        case "verbosity":
            cfg.Engine.Verbosity = *verbosity
        case "pretty":
            cfg.Output.Pretty = *pretty
        case "log-level":
            cfg.LogLevel = *logLevel
        case "from":
            if t, err := time.Parse(time.RFC3339, *from); err == nil {
                cfg.QueryFrom = t
            }
        case "to":
            if t, err := time.Parse(time.RFC3339, *to); err == nil {
                cfg.QueryTo = t
            }
        case "limit":
            cfg.QueryLimit = *limit
        }
    })

    return cfg
}
```

6.3 **Add Mode validation** to `Validate()`:

```go
switch c.Mode {
case "stream", "query", "":
    // valid
default:
    errs = append(errs, fmt.Sprintf("invalid mode %q (must be stream or query)", c.Mode))
}
```

6.4 **Update `main.go`** — use `LoadWithFlags()`, add stream/query mode switch:

```go
cfg := config.LoadWithFlags()
logging.Init(cfg.Output.Format == "stdout", logging.ParseLevel(cfg.LogLevel))

// ... validation, component init, pipeline setup ...

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
        slog.Error("query failed", "error", err)
        os.Exit(1)
    }
default: // "stream"
    slog.Info("starting stream", "connector", cfg.Connector.Provider)
    if err := p.Stream(ctx, connCfg); err != nil && err != context.Canceled {
        slog.Error("pipeline error", "error", err)
        os.Exit(1)
    }
}
```

### Files

| File | Action |
|------|--------|
| `internal/config/config.go` | Add Mode/QueryFrom/QueryTo/QueryLimit fields, LoadWithFlags(), Mode validation in Validate() |
| `internal/config/config_test.go` | Add mode tests |
| `cmd/lumber/main.go` | Use LoadWithFlags(), add stream/query mode switch |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestLoad_ModeDefault` | Default mode is "stream" |
| `TestLoad_ModeEnv` | `LUMBER_MODE=query` sets mode |
| `TestValidate_BadMode` | `"replay"` → error containing "mode" |

Note: `LoadWithFlags()` calls `flag.Parse()` which modifies global state. Flag override logic is tested via the `flag.Visit` pattern in isolation; `LoadWithFlags()` itself is integration-tested via `go build` + manual invocation.

### Verification

```
go test ./internal/config/...
go build ./cmd/lumber
./lumber -help
```

---

## Section 7: End-to-End Integration Test

**What:** Full pipeline test: httptest server → Vercel connector → real ONNX engine → mock output. Proves the pieces work as a system.

### Tasks

7.1 **Create `internal/pipeline/integration_test.go`** (~150 lines).

Guarded by `skipWithoutModel(t)` — skips when ONNX model files are not present. Follows the same pattern as `internal/engine/engine_test.go`.

Setup:
- Start `httptest.NewServer` serving Vercel-format JSON responses with realistic log messages
- Create real engine: `embedder.New()` → `taxonomy.New()` → `classifier.New()` → `compactor.New()` → `engine.New()`
- Create `mockOutput` to capture events (reuse existing mock pattern from `pipeline_test.go`)
- Create Vercel connector pointed at test server via `ConnectorConfig.Endpoint`

### Files

| File | Action |
|------|--------|
| `internal/pipeline/integration_test.go` | New (~150 lines) |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestIntegration_VercelStreamThroughPipeline` | Stream 3 realistic logs (connection error, HTTP 200, slow query) through httptest → real engine → mock output. Verify: correct count, non-empty Type/Category/Severity/Summary, confidence > 0 |
| `TestIntegration_VercelQueryThroughPipeline` | Query mode with time range. Verify batch classification produces correct events |
| `TestIntegration_BadLogDoesNotCrash` | Mix valid logs with edge cases (empty string, binary content). Verify pipeline continues without crashing (Section 3 resilience) |
| `TestIntegration_DedupReducesCount` | Send 10 identical error logs. With dedup enabled, output should have fewer events with Count > 1 |

### Verification

```
go test -v -run Integration ./internal/pipeline/...   # requires ONNX model
go test ./internal/pipeline/...                        # skips integration without model
```

---

## Implementation Order

```
Section 1: Structured Logging (foundation — all other sections use slog)
    │
    ├──→ Section 2: Config Validation (uses slog for error reporting)
    │
    ├──→ Section 3: Per-Log Error Handling (uses slog.Warn for skip logging)
    │       │
    │       └──→ Section 4: Bounded Buffer (builds on pipeline changes from Section 3)
    │
    ├──→ Section 5: Graceful Shutdown (uses slog, touches main.go + pipeline)
    │
    ├──→ Section 6: CLI Flags (touches config + main.go)
    │
    └──→ Section 7: Integration Tests (validates all above)
```

Recommended sequence: **1 → 2 → 3 → 4 → 5 → 6 → 7**

---

## New Environment Variables

| Variable | Default | Type | Description |
|----------|---------|------|-------------|
| `LUMBER_LOG_LEVEL` | `info` | string | Internal log level: debug, info, warn, error |
| `LUMBER_SHUTDOWN_TIMEOUT` | `10s` | duration | Max time to drain in-flight logs on shutdown |
| `LUMBER_MAX_BUFFER_SIZE` | `1000` | int | Max events buffered before force dedup flush; 0 = unlimited |
| `LUMBER_MODE` | `stream` | string | Pipeline mode: stream or query |

## New CLI Flags

| Flag | Type | Description |
|------|------|-------------|
| `-mode` | string | Pipeline mode: stream or query (overrides `LUMBER_MODE`) |
| `-connector` | string | Connector provider (overrides `LUMBER_CONNECTOR`) |
| `-from` | string | Query start time, RFC3339 |
| `-to` | string | Query end time, RFC3339 |
| `-limit` | int | Query result limit |
| `-verbosity` | string | Output verbosity (overrides `LUMBER_VERBOSITY`) |
| `-pretty` | bool | Pretty-print JSON (overrides `LUMBER_OUTPUT_PRETTY`) |
| `-log-level` | string | Log level (overrides `LUMBER_LOG_LEVEL`) |

## Files Summary

| File | Sections | Action |
|------|----------|--------|
| `internal/logging/logging.go` | 1 | New (~30 lines) |
| `internal/logging/logging_test.go` | 1 | New (~40 lines, 3 tests) |
| `internal/config/config.go` | 1,2,4,5,6 | Modified — new fields, Validate(), LoadWithFlags(), getenvInt() |
| `internal/config/config_test.go` | 2,4,5,6 | Modified — ~12 new tests |
| `internal/pipeline/pipeline.go` | 3,4,5 | Modified — Processor interface, skip logic, atomic counter, buffer option, flush fix |
| `internal/pipeline/buffer.go` | 4 | Modified — maxSize, bool return from add() |
| `internal/pipeline/pipeline_test.go` | 3,4 | Modified — error handling + buffer tests |
| `internal/pipeline/integration_test.go` | 7 | New (~150 lines, 4 tests) |
| `internal/connector/vercel/vercel.go` | 1 | Modified — slog.Warn |
| `internal/connector/flyio/flyio.go` | 1 | Modified — slog.Warn |
| `internal/connector/supabase/supabase.go` | 1 | Modified — slog.Warn (×2) |
| `cmd/lumber/main.go` | 1,2,5,6 | Modified — logging, validation, shutdown, flags, mode switch |

**New files: 3. Modified files: 9. Total: 12.**
**Estimated new tests: ~25** (3 logging + 7 validation + 4 error handling + 4 buffer + 3 config + 4 integration)

## Full Verification

```
go test ./...                                          # all non-ONNX tests
go test -v -run Integration ./internal/pipeline/...    # ONNX integration tests
go build ./cmd/lumber                                  # binary compiles
./lumber -help                                         # flags visible
```

---

## What's Explicitly Not In Scope

- **Additional output destinations** (file, webhook, gRPC) — Output interface is extensible, no new implementations for Phase 5
- **Metrics/Prometheus export** — structured logging provides observability; metrics deferred to post-beta
- **Log rotation** — Lumber logs to stderr; rotation is the host's concern
- **Configuration files** (YAML/TOML) — env vars + CLI flags are sufficient for beta
- **Per-field CLI flags** for connector config — connector-specific config stays in env vars (`LUMBER_VERCEL_PROJECT_ID` etc.)
- **Dedup persistence across restarts** — in-memory only
- **Backpressure propagation to connectors** — connector channels remain fixed at 64; bounded dedup buffer handles the pipeline side
