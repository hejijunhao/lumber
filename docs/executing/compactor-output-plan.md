# Phase 4: Compactor & Output Hardening — Implementation Plan

## Goal

Make Lumber's compaction genuinely token-efficient and the output production-quality. Fix UTF-8 safety bugs, add stack-trace-aware truncation, strip high-cardinality structured fields, implement batch-level event deduplication with counted summaries, add JSON output formatting options, and validate efficiency gains with token-count measurement. All new code covered by unit tests that run without ONNX model files.

**Success criteria:**
- UTF-8 safe truncation and summarization (no split multi-byte characters)
- Stack trace detection with first/last frame preservation
- JSON structured field stripping (trace IDs, request IDs) at Minimal/Standard verbosity
- Event deduplication with counted summaries for both Query and Stream modes
- Compact and pretty JSON output with lowercase field names
- Verbosity-aware field omission (Raw and Confidence stripped at Minimal)
- Token count measurement validating compaction ratios in tests
- `go build ./cmd/lumber` compiles, all new tests pass

---

## Current State

**Working:**
- `Compactor.Compact(raw string) (compacted, summary string)` — verbosity-driven truncation (200/2000/unlimited chars)
- `summarize()` — first 120 characters + "..."
- `Verbosity` enum: Minimal, Standard, Full
- `CanonicalEvent` — no JSON tags (Go-default uppercase field names in output)
- `stdout.Output` — NDJSON to stdout, no formatting options
- `OutputConfig.Format` — only "stdout"
- `LUMBER_VERBOSITY` env var wired through config → compactor

**Known bugs:**
- `truncate()` slices on byte index — splits multi-byte UTF-8
- `summarize()` slices on byte index — same bug

**Not yet built:**
- No stack trace detection or smart truncation
- No structured field stripping
- No deduplication (per-event or batch-level)
- No JSON tags on `CanonicalEvent`
- No compact/pretty output toggle
- No verbosity-aware field omission in output
- No token count measurement
- Zero compactor unit tests

---

## Section 1: Per-Event Compaction

**What:** Fix UTF-8 bugs, add stack-trace-aware truncation, structured field stripping, and intelligent summarization. Update `Compact` signature to accept event type for type-aware logic.

### Tasks

1.1 **Update `Compact` signature** from `Compact(raw string)` to `Compact(raw, eventType string)`. The engine already has the classification result when it calls Compact, so passing the type is trivial. Update call sites in `engine.go`.

1.2 **Replace byte-index truncation with rune-aware truncation.** Use `range` iteration to find the cut point at a rune boundary. Keep the `"..."` suffix.

1.3 **Replace byte-index summarize with first-line extraction.** New logic: extract the first line (up to `\n`), trim to 120 runes at a word boundary, append `"..."` if truncated. Produces a meaningful one-liner instead of arbitrary bytes.

1.4 **Add stack-trace-aware truncation.** New unexported function `truncateStackTrace(raw string, maxLines int) string` that:
- Detects stack trace patterns: lines starting with `\tat ` (Java), `goroutine ` (Go), or matching `^\s+at .+:\d+` or `^\s+.+\.go:\d+`
- Preserves the first `maxLines` frames and last 2 frames
- Replaces the middle with `... (N frames omitted) ...`
- Called from `Compact` when a stack trace pattern is detected in the raw text
- Default `maxLines`: 5 at Minimal, 10 at Standard, unlimited at Full

1.5 **Add structured field stripping.** New unexported function `stripFields(raw string, fields []string) string` that:
- Detects JSON-formatted log lines (starts with `{`)
- Parses as `map[string]any`, removes keys matching the strip list, re-serializes compactly
- Non-JSON logs pass through unchanged
- Default strip list: `["trace_id", "span_id", "request_id", "x_request_id", "correlation_id", "dd.trace_id", "dd.span_id"]`
- Called at Minimal and Standard verbosity, skipped at Full
- Strip list is a `[]string` field on `Compactor`, set via constructor option

1.6 **Update `Compactor` struct and `New` constructor:**
```go
type Compactor struct {
    Verbosity  Verbosity
    StripFields []string // high-cardinality fields to remove from JSON logs
}

func New(v Verbosity, opts ...Option) *Compactor
type Option func(*Compactor)
func WithStripFields(fields []string) Option
```

Default strip list applied when no `WithStripFields` option is provided. `Compact` flow becomes: strip fields → truncate (stack-trace-aware for errors) → summarize.

### Files

| File | Action |
|------|--------|
| `internal/engine/compactor/compactor.go` | Rewrite: rune-safe truncation, first-line summarize, stack trace handling, field stripping, updated signature and constructor |
| `internal/engine/compactor/compactor_test.go` | New: comprehensive unit tests |
| `internal/engine/engine.go` | Update `Compact` call sites to pass `eventType` |

### Verification

```
go test ./internal/engine/compactor/...
```

Tests (no ONNX required):
- `TestTruncateRuneSafety` — multi-byte (CJK, emoji) at boundary, result is valid UTF-8
- `TestTruncateASCII` — ASCII strings behave as before
- `TestSummarizeFirstLine` — multi-line input uses first line
- `TestSummarizeWordBoundary` — long first line truncated at word boundary
- `TestSummarizeShortInput` — under 120 runes returned unchanged
- `TestStackTraceJava` — 30-frame Java trace reduced to 5+2 with omission message
- `TestStackTraceGo` — Go goroutine dump handled similarly
- `TestStackTraceNone` — non-trace ERROR logs truncated normally
- `TestStripFieldsJSON` — JSON log with trace_id/request_id has them removed
- `TestStripFieldsNonJSON` — plain text passes through unchanged
- `TestStripFieldsFullVerbosity` — no stripping at Full
- `TestStripFieldsCustomList` — custom strip list via option
- `TestCompactMinimal` — full flow at Minimal verbosity
- `TestCompactStandard` — full flow at Standard
- `TestCompactFull` — Full preserves everything, no stripping

---

## Section 2: JSON Output Formatting

**What:** Add JSON tags to `CanonicalEvent` for clean output, implement verbosity-aware field omission, and add compact/pretty JSON formatting toggle.

### Tasks

2.1 **Add JSON tags and `Count` field to `CanonicalEvent`:**
```go
type CanonicalEvent struct {
    Type       string    `json:"type"`
    Category   string    `json:"category"`
    Severity   string    `json:"severity"`
    Timestamp  time.Time `json:"timestamp"`
    Summary    string    `json:"summary"`
    Confidence float64   `json:"confidence,omitempty"`
    Raw        string    `json:"raw,omitempty"`
    Count      int       `json:"count,omitempty"` // >0 when deduplicated
}
```

`Count` defaults to 0 (omitted from JSON via `omitempty`). Set by dedup in Section 3. `Confidence` and `Raw` use `omitempty` so they disappear when zero-valued — leveraged by field omission below.

2.2 **Create `FormatEvent` function** in `internal/output/format.go`:
```go
// FormatEvent returns a copy of the event with fields stripped according to verbosity.
// At Minimal: Raw and Confidence are zeroed (omitted from JSON via omitempty).
// At Standard/Full: all fields preserved.
func FormatEvent(e model.CanonicalEvent, verbosity compactor.Verbosity) model.CanonicalEvent
```

Returns a `CanonicalEvent` (not a new type) with fields zeroed for omission. This avoids introducing a second event struct while leveraging `omitempty` tags.

2.3 **Update `stdout.Output`** to accept verbosity and pretty options:
```go
func New(verbosity compactor.Verbosity, pretty bool) *Output
```

Constructor configures `json.Encoder.SetIndent("", "  ")` when `pretty` is true. `Write` calls `FormatEvent` before encoding.

2.4 **Add `Pretty` to config:** `LUMBER_OUTPUT_PRETTY` env var (default `false`), parsed as bool in `OutputConfig`.

2.5 **Update `cmd/lumber/main.go`** to pass verbosity and pretty to `stdout.New`.

### Files

| File | Action |
|------|--------|
| `internal/model/event.go` | Add JSON tags, add `Count` field |
| `internal/output/format.go` | New: `FormatEvent` function |
| `internal/output/format_test.go` | New: format tests |
| `internal/output/stdout/stdout.go` | Update constructor, integrate FormatEvent, pretty support |
| `internal/output/stdout/stdout_test.go` | New: output format tests |
| `internal/config/config.go` | Add `Pretty bool` to `OutputConfig`, read env var |
| `internal/config/config_test.go` | Add pretty config test |
| `cmd/lumber/main.go` | Pass verbosity + pretty to `stdout.New` |

### Verification

```
go test ./internal/output/... ./internal/config/...
```

Tests:
- `TestFormatEventMinimal` — Raw and Confidence are zeroed
- `TestFormatEventStandard` — all fields preserved
- `TestFormatEventFull` — all fields preserved
- `TestFormatEventCount` — Count > 0 preserved, Count == 0 omitted from JSON
- `TestOutputCompactJSON` — single-line NDJSON with lowercase keys
- `TestOutputPrettyJSON` — indented multi-line JSON
- `TestJSONTagNames` — marshal CanonicalEvent, verify all keys lowercase
- `TestConfigPrettyDefault` — default false
- `TestConfigPrettyEnv` — `LUMBER_OUTPUT_PRETTY=true` parsed correctly

---

## Section 3: Event Deduplication

**What:** Batch-level deduplication that collapses identical event types into counted summaries. A new `internal/engine/dedup` package that operates on classified `CanonicalEvent` values at the pipeline level — after engine, before output.

### Tasks

3.1 **Create `internal/engine/dedup/dedup.go`:**
```go
type Config struct {
    Window time.Duration // grouping window (default 5s)
}

type Deduplicator struct { cfg Config }

func New(cfg Config) *Deduplicator

// DeduplicateBatch collapses events with identical Type+Category
// within Window of each other. Returns events in first-occurrence order.
// Sets Count on merged events and rewrites Summary to include count.
func (d *Deduplicator) DeduplicateBatch(events []model.CanonicalEvent) []model.CanonicalEvent
```

3.2 **Deduplication logic.** Iterate events in order. Dedup key: `Type + "." + Category`. Maintain ordered map (slice of keys + map to accumulator struct). When a key is seen again and timestamp is within `Window` of the group's first timestamp, increment count. When outside the window, start a new group for the same key. Output: one event per group with `Count` set (only if > 1) and `Summary` rewritten to `"<original_summary> (x<count> in <duration>)"`. Preserves first event's timestamp.

3.3 **Wire into pipeline.** Add optional `*dedup.Deduplicator` field to `Pipeline`:
```go
func New(conn connector.Connector, eng *engine.Engine, out output.Output, opts ...Option) *Pipeline
type Option func(*Pipeline)
func WithDedup(d *dedup.Deduplicator) Option
```

In `Query()`: after `ProcessBatch`, call `DeduplicateBatch` before writing. In `Stream()`: use a windowed buffer (task 3.4).

3.4 **Streaming dedup buffer.** New `internal/pipeline/buffer.go` with a `streamBuffer` type:
- Events accumulate in a slice protected by a mutex
- First event starts a timer for `Window` duration
- When timer fires: deduplicate pending events, write all to output, reset buffer
- On context cancellation: flush remaining events immediately
- `Pipeline.Stream()` uses the buffer when dedup is configured, falls through to direct write when not

3.5 **Config wiring.** Add `LUMBER_DEDUP_WINDOW` env var (default `"5s"`, `"0"` to disable). Parse as `time.Duration`. When 0, dedup is nil and pipeline skips it.

3.6 **Update `cmd/lumber/main.go`** to create `Deduplicator` from config and pass to pipeline via `WithDedup`.

### Files

| File | Action |
|------|--------|
| `internal/engine/dedup/dedup.go` | New: Deduplicator and DeduplicateBatch |
| `internal/engine/dedup/dedup_test.go` | New: unit tests |
| `internal/pipeline/pipeline.go` | Add optional dedup, wire into Query and Stream |
| `internal/pipeline/buffer.go` | New: streaming dedup buffer |
| `internal/pipeline/pipeline_test.go` | New: pipeline dedup integration tests |
| `internal/config/config.go` | Add `DedupWindow time.Duration` to `EngineConfig` |
| `internal/config/config_test.go` | Add dedup config tests |
| `cmd/lumber/main.go` | Create Deduplicator, pass to pipeline |

### Verification

```
go test ./internal/engine/dedup/... ./internal/pipeline/...
```

Tests:
- `TestDeduplicateBatchEmpty` — empty → empty
- `TestDeduplicateBatchNoDuplicates` — distinct events unchanged, Count unset
- `TestDeduplicateBatchSimple` — 5 identical collapse to 1 with Count=5
- `TestDeduplicateBatchMixed` — `[A, B, A, A, B]` → `[A(x3), B(x2)]` in first-occurrence order
- `TestDeduplicateBatchWindowExpiry` — events spanning > Window produce separate groups for same key
- `TestDeduplicateBatchSummaryFormat` — summary contains `"(x47 in 4m32s)"`
- `TestDeduplicateBatchPreservesTimestamp` — uses earliest timestamp
- `TestStreamBufferFlush` — 10 events within window, flushed as deduplicated batch after window
- `TestStreamBufferContextCancel` — cancel mid-window, remaining flushed
- `TestQueryWithDedup` — mock engine + mock output, dedup applied
- `TestPipelineWithoutDedup` — nil dedup, events pass through directly

---

## Section 4: Token Measurement & Validation

**What:** Token estimation utility for measuring compaction efficiency, and integration tests that validate ratios across all verbosity levels with realistic log inputs.

### Tasks

4.1 **Create `internal/engine/compactor/tokencount.go`:**
```go
// EstimateTokens returns an approximate token count using a whitespace heuristic.
// Splits on whitespace, applies a 1.3x subword expansion factor (rounded up).
// Not a real tokenizer — accurate within ~20% of BPE counts, sufficient for
// measuring compaction ratios.
func EstimateTokens(s string) int
```

4.2 **Create integration test file** `internal/engine/compactor/integration_test.go` with realistic log inputs:
- JSON structured log with trace IDs (~500 bytes)
- Java stack trace (~2KB, 30 frames)
- Go panic with goroutine dump (~1.5KB)
- Plain text multi-line error (~300 bytes)
- Short single-line request log (~80 bytes)

For each input, test all three verbosity levels and assert:
- Output is valid UTF-8
- Token count after < token count before (at Minimal/Standard)
- Stack traces are properly reduced
- Field stripping removes expected keys
- Summaries are meaningful first-line extractions

### Files

| File | Action |
|------|--------|
| `internal/engine/compactor/tokencount.go` | New: EstimateTokens |
| `internal/engine/compactor/tokencount_test.go` | New: estimation tests |
| `internal/engine/compactor/integration_test.go` | New: end-to-end compaction tests with realistic logs |

### Verification

```
go test ./internal/engine/compactor/...
```

Tests:
- `TestEstimateTokensEmpty` — returns 0
- `TestEstimateTokensSimple` — `"hello world"` → ~3
- `TestEstimateTokensLong` — 500-word paragraph → reasonable count
- `TestIntegrationMinimalStackTrace` — Java trace at Minimal: truncated, stripped, summary meaningful, token reduction > 60%
- `TestIntegrationStandardStructuredLog` — JSON at Standard: trace IDs stripped, raw preserved (up to 2000 runes)
- `TestIntegrationFullPreservesEverything` — Full: input unchanged
- `TestIntegrationMultibyteUTF8` — CJK logs at all levels produce valid UTF-8

---

## Implementation Order

```
Section 1: Per-Event Compaction (UTF-8, stack trace, field stripping)
    │
    ├──→ Section 2: JSON Output Formatting (tags, pretty, field omission)
    │        │
    │        └──→ Section 3: Event Deduplication (Count field from Section 2)
    │
    └──→ Section 4: Token Measurement & Validation (uses Section 1 compactor)
```

Section 1 first (foundational compactor rewrite). Section 2 can start once Section 1's `Compact` signature change is in, but touches different files (output/model vs compactor). Section 3 depends on the `Count` field added in Section 2. Section 4 depends on Section 1's compactor being functional.

---

## Files Summary

| File | Section | Action |
|------|---------|--------|
| `internal/engine/compactor/compactor.go` | 1 | Major rewrite |
| `internal/engine/compactor/compactor_test.go` | 1 | New |
| `internal/engine/compactor/tokencount.go` | 4 | New |
| `internal/engine/compactor/tokencount_test.go` | 4 | New |
| `internal/engine/compactor/integration_test.go` | 4 | New |
| `internal/engine/engine.go` | 1 | Update Compact call sites |
| `internal/model/event.go` | 2 | Add JSON tags + Count field |
| `internal/output/format.go` | 2 | New |
| `internal/output/format_test.go` | 2 | New |
| `internal/output/stdout/stdout.go` | 2 | Update constructor + formatting |
| `internal/output/stdout/stdout_test.go` | 2 | New |
| `internal/engine/dedup/dedup.go` | 3 | New |
| `internal/engine/dedup/dedup_test.go` | 3 | New |
| `internal/pipeline/pipeline.go` | 3 | Add optional dedup |
| `internal/pipeline/buffer.go` | 3 | New |
| `internal/pipeline/pipeline_test.go` | 3 | New |
| `internal/config/config.go` | 2, 3 | Add Pretty + DedupWindow |
| `internal/config/config_test.go` | 2, 3 | Add config tests |
| `cmd/lumber/main.go` | 2, 3 | Wire new options |

**New files: 11. Modified files: 8. Total: 19.**

---

## What's Explicitly Not In Scope

- **Real BPE tokenizer** — whitespace heuristic is sufficient for ratio measurement
- **Dedup persistence across restarts** — in-memory only, no state survives restart
- **Output destinations beyond stdout** — Output interface is extensible, but no new implementations
- **Configurable stack trace patterns** — hardcoded Java/Go detection, extensible later
- **Streaming dedup backpressure** — buffer grows unboundedly during window; bounded buffer deferred to Phase 5
- **Per-field verbosity control** — single global verbosity setting
- **Metrics export** — token counts logged to stderr, not Prometheus/etc.
- **Buffering and graceful shutdown** — Phase 5 concern
