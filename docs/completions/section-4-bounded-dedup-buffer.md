# Section 4: Bounded Dedup Buffer — Completion Notes

**Completed:** 2026-02-23
**Phase:** 5 (Pipeline Integration & Resilience)
**Depends on:** Section 3 (Per-Log Error Handling) — builds on the pipeline changes from Section 3

## Summary

Added a `maxSize` ceiling to the dedup `streamBuffer`. When the buffer hits `maxSize` events, it force-flushes immediately instead of waiting for the timer. No events are dropped — they're just deduplicated and written sooner. This prevents unbounded memory growth during log storms.

## What Changed

### Modified Files

| File | Change | Why |
|------|--------|-----|
| `internal/pipeline/buffer.go` | Added `maxSize int` field; updated `newStreamBuffer()` to take 4th `maxSize` parameter; changed `add()` return type from void to `bool` (true when buffer is full) | Core bounded buffer logic |
| `internal/pipeline/pipeline.go` | Added `maxBufferSize int` field to `Pipeline`; added `WithMaxBufferSize(n int) Option`; `streamWithDedup()` passes `p.maxBufferSize` to buffer constructor; handles `buf.add()` bool return with early flush | Wiring the option through to the buffer |
| `internal/config/config.go` | Added `MaxBufferSize int` to `EngineConfig`; reads `LUMBER_MAX_BUFFER_SIZE` env var (default 1000); added `getenvInt()` helper | Config plumbing |
| `internal/config/config_test.go` | Added `TestGetenvInt` (5 sub-tests), `TestLoad_MaxBufferSizeDefault`, `TestLoad_MaxBufferSizeEnv` | Config coverage |
| `internal/pipeline/pipeline_test.go` | Updated 3 existing `newStreamBuffer()` calls to pass `0` (unlimited); added 3 new buffer tests | Buffer behavior coverage |
| `cmd/lumber/main.go` | Added `pipeline.WithMaxBufferSize(cfg.Engine.MaxBufferSize)` when > 0 | Binary wiring |

### `add()` Signature Change

Before:
```go
func (b *streamBuffer) add(event model.CanonicalEvent)
```

After:
```go
func (b *streamBuffer) add(event model.CanonicalEvent) bool
```

Returns `true` when `maxSize > 0 && len(b.pending) >= maxSize`. The caller in `streamWithDedup()` checks this and calls `buf.flush(ctx)` immediately.

### Tests Added (6)

| Test | File | What it validates |
|------|------|-------------------|
| `TestStreamBuffer_MaxSizeFlush` | pipeline_test.go | `add()` returns false for events 1-4, true on event 5 (maxSize=5) |
| `TestStreamBuffer_MaxSizeNoDataLoss` | pipeline_test.go | Fill to 3, flush, add 2 more, flush — all 5 events in output |
| `TestStreamBuffer_UnlimitedBackcompat` | pipeline_test.go | maxSize=0 allows 10,000 events without returning true |
| `TestGetenvInt` | config_test.go | 5 sub-tests: empty/valid/zero/invalid/negative |
| `TestLoad_MaxBufferSizeDefault` | config_test.go | Default is 1000 |
| `TestLoad_MaxBufferSizeEnv` | config_test.go | `LUMBER_MAX_BUFFER_SIZE=500` parsed correctly |

## Design Decisions

- **`add()` returns bool rather than flushing internally** — flush needs `context.Context` and returns an error. Keeping `add()` side-effect-free lets the caller handle error propagation in the select loop.
- **Default 1000, not unlimited** — a 5s dedup window at 1000 logs/sec means 5000 buffered events. Capping at 1000 means at most ~1000 events in memory before a force flush. This is a reasonable default that prevents runaway memory while preserving dedup effectiveness.
- **maxSize=0 preserves existing behavior** — backward compatible for tests and code that doesn't set the option.

## New Environment Variable

| Variable | Default | Type | Description |
|----------|---------|------|-------------|
| `LUMBER_MAX_BUFFER_SIZE` | `1000` | int | Max events buffered before force dedup flush; 0 = unlimited |

## Verification

```
go test ./internal/pipeline/...  # 13 tests pass (7 existing + 3 buffer + 3 already from S3)
go test ./internal/config/...    # 21 tests pass (16 existing + 5 new)
go build ./cmd/lumber            # compiles cleanly
go test ./...                    # full suite passes
```
