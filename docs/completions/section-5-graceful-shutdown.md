# Section 5: Graceful Shutdown — Completion Notes

**Completed:** 2026-02-23
**Phase:** 5 (Pipeline Integration & Resilience)
**Depends on:** Section 1 (Structured Logging) — uses slog in signal handler

## Summary

Three fixes to prevent the process from hanging on shutdown:

1. **Shutdown timeout** — configurable `ShutdownTimeout` (default 10s) with force `os.Exit(1)` if drain exceeds it.
2. **Double-signal force exit** — second SIGINT/SIGTERM forces immediate exit, matching standard Unix service behavior.
3. **Background context for final flush** — `streamWithDedup` now uses `context.Background()` for the drain flush instead of the already-cancelled `ctx`, so `output.Write` calls can complete. The shutdown timer in main provides the hard bound.

## What Changed

### Modified Files

| File | Change | Why |
|------|--------|-----|
| `internal/config/config.go` | Added `ShutdownTimeout time.Duration` to `Config`; reads `LUMBER_SHUTDOWN_TIMEOUT` (default 10s) | Configurable drain timeout |
| `internal/config/config_test.go` | Added `TestLoad_ShutdownTimeoutDefault` and `TestLoad_ShutdownTimeoutEnv` | Coverage for new field |
| `cmd/lumber/main.go` | Signal channel buffered to 2; added shutdown timer goroutine with double-signal and timeout branches; logs timeout value on first signal | Bounded shutdown + force exit |
| `internal/pipeline/pipeline.go` | Changed `buf.flush(ctx)` to `buf.flush(context.Background())` in the `ctx.Done()` case of `streamWithDedup`; updated error message to "pipeline flush on shutdown" | Prevents flush failure from cancelled context |

### Signal Handler (main.go)

Before:
```go
sigCh := make(chan os.Signal, 1)
// ...
sig := <-sigCh
cancel()
```

After:
```go
sigCh := make(chan os.Signal, 2) // buffer 2 to catch second signal
// ...
sig := <-sigCh
cancel()

timer := time.NewTimer(cfg.ShutdownTimeout)
select {
case sig := <-sigCh:       // second signal → force exit
    os.Exit(1)
case <-timer.C:            // timeout exceeded → force exit
    os.Exit(1)
}
```

The goroutine exits naturally if the main goroutine finishes draining before the timer fires (process exits, goroutine is cleaned up).

### Flush Context Fix (pipeline.go)

Before:
```go
case <-ctx.Done():
    if err := buf.flush(ctx); err != nil {  // ctx is already cancelled!
```

After:
```go
case <-ctx.Done():
    if err := buf.flush(context.Background()); err != nil {  // can complete writes
```

### Tests Added (2)

| Test | What it validates |
|------|-------------------|
| `TestLoad_ShutdownTimeoutDefault` | Default is 10s |
| `TestLoad_ShutdownTimeoutEnv` | `LUMBER_SHUTDOWN_TIMEOUT=5s` parsed correctly |

Signal handling (double-signal, timeout) is inherently difficult to unit test (requires OS signals and process lifecycle). Verified manually.

## Design Decisions

- **`context.Background()` for drain flush, not a new timeout context** — the shutdown timer in `main.go` already provides the hard bound via `os.Exit(1)`. Creating a second timeout context for the flush would duplicate that logic. Background context keeps the pipeline layer simple.
- **Buffer size 2 on signal channel** — ensures the second signal is captured even if the goroutine hasn't started reading yet.
- **10s default timeout** — long enough for a typical dedup flush (5s window + write latency) but short enough that users don't think the process is hung.

## New Environment Variable

| Variable | Default | Type | Description |
|----------|---------|------|-------------|
| `LUMBER_SHUTDOWN_TIMEOUT` | `10s` | duration | Max time to drain in-flight logs on shutdown |

## Verification

```
go test ./internal/config/...    # 23 tests pass
go test ./internal/pipeline/...  # 13 tests pass
go build ./cmd/lumber            # compiles cleanly
go test ./...                    # full suite passes
```
