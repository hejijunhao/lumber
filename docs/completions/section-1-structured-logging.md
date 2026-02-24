# Section 1: Structured Internal Logging — Completion Notes

**Completed:** 2026-02-23
**Phase:** 5 (Pipeline Integration & Resilience)

## Summary

Replaced three inconsistent logging patterns (`fmt.Fprintf(os.Stderr)`, `log.Printf`, `log.Fatalf`) with `log/slog` (stdlib since Go 1.21). All subsequent Phase 5 sections depend on this structured logger being in place.

## What Changed

### New Files

| File | Lines | Purpose |
|------|-------|---------|
| `internal/logging/logging.go` | ~35 | `Init(outputIsStdout, level)` and `ParseLevel(s)` — configures the global slog default |
| `internal/logging/logging_test.go` | ~70 | 3 tests: ParseLevel mapping, JSON handler output, Text handler output |

### Modified Files

| File | Change | Why |
|------|--------|-----|
| `internal/config/config.go` | Added `LogLevel string` to `Config`, read from `LUMBER_LOG_LEVEL` (default `"info"`) | Process-wide log level setting; not nested in Engine/Output because it applies globally |
| `cmd/lumber/main.go` | Replaced `fmt`/`log` imports with `log/slog` + `logging` package. Called `logging.Init()` after `config.Load()`. Replaced 5× `fmt.Fprintf(os.Stderr, ...)` with `slog.Info(...)`. Replaced 3× `log.Fatalf(...)` with `slog.Error(...) + os.Exit(1)` | Structured, leveled, consistent logging throughout the entrypoint |
| `internal/connector/vercel/vercel.go` | `log.Printf("vercel connector: poll error: %v", err)` → `slog.Warn("poll error", "connector", "vercel", "error", err)` | Structured fields instead of string interpolation |
| `internal/connector/flyio/flyio.go` | Same pattern as vercel | Consistency |
| `internal/connector/supabase/supabase.go` | 2× `log.Printf(...)` → `slog.Warn(...)` with `"connector"`, `"table"`, `"error"` fields | Per-table error isolation is now queryable by field |

## Design Decisions

- **`slog.SetDefault()` over dependency injection** — call sites use bare `slog.Info(...)` etc. No logger parameter threading through the call stack. The handler always writes to `os.Stderr`.
- **JSON handler when output is stdout** — prevents mixing machine-parseable NDJSON pipeline output with human-readable log text. Both stderr (logs) and stdout (events) are machine-parseable.
- **Text handler otherwise** — for development/debugging, human-readable format is easier to scan.
- **`ParseLevel` defaults to `LevelInfo` for unknown strings** — fail-open to reasonable default rather than requiring exact string match.

## Mapping Table

| Before | After |
|--------|-------|
| `fmt.Fprintf(os.Stderr, "lumber: embedder loaded model=%s dim=%d\n", ...)` | `slog.Info("embedder loaded", "model", cfg.Engine.ModelPath, "dim", emb.EmbedDim())` |
| `fmt.Fprintf(os.Stderr, "lumber: taxonomy pre-embedded %d labels in %s\n", ...)` | `slog.Info("taxonomy pre-embedded", "labels", len(tax.Labels()), "duration", time.Since(t0).Round(time.Millisecond))` |
| `fmt.Fprintf(os.Stderr, "lumber: dedup enabled window=%s\n", ...)` | `slog.Info("dedup enabled", "window", cfg.Engine.DedupWindow)` |
| `fmt.Fprintf(os.Stderr, "\nreceived %v, shutting down...\n", sig)` | `slog.Info("shutting down", "signal", sig)` |
| `fmt.Fprintf(os.Stderr, "lumber: starting with connector=%s\n", ...)` | `slog.Info("starting", "connector", cfg.Connector.Provider)` |
| `log.Fatalf("failed to create embedder: %v", err)` | `slog.Error("failed to create embedder", "error", err); os.Exit(1)` |
| `log.Fatalf("failed to create taxonomy: %v", err)` | `slog.Error("failed to create taxonomy", "error", err); os.Exit(1)` |
| `log.Fatalf("failed to get connector: %v", err)` | `slog.Error("failed to get connector", "error", err); os.Exit(1)` |
| `log.Fatalf("pipeline error: %v", err)` | `slog.Error("pipeline error", "error", err); os.Exit(1)` |
| `log.Printf("vercel connector: poll error: %v", err)` | `slog.Warn("poll error", "connector", "vercel", "error", err)` |
| `log.Printf("flyio connector: poll error: %v", err)` | `slog.Warn("poll error", "connector", "flyio", "error", err)` |
| `log.Printf("supabase connector: %v", err)` | `slog.Warn("sql build error", "connector", "supabase", "table", table, "error", err)` |
| `log.Printf("supabase connector: poll error (%s): %v", table, err)` | `slog.Warn("poll error", "connector", "supabase", "table", table, "error", err)` |

## New Environment Variable

| Variable | Default | Type | Description |
|----------|---------|------|-------------|
| `LUMBER_LOG_LEVEL` | `info` | string | Internal log level: debug, info, warn, error |

## Verification

```
go test ./internal/logging/...   # 3 tests pass
go test ./internal/config/...    # 9 tests pass (existing + no new yet for LogLevel)
go test ./internal/connector/... # 34 tests pass (unchanged behavior)
go test ./...                    # all 104 tests pass
go build ./cmd/lumber            # compiles cleanly
```
