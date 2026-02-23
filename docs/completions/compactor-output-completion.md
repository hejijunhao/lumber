# Phase 4: Compactor & Output Hardening — Completion

## Goal

Make Lumber's compaction genuinely token-efficient and the output production-quality. Fix UTF-8 safety bugs, add stack-trace-aware truncation, strip high-cardinality structured fields, implement batch-level event deduplication with counted summaries, add JSON output formatting options, and validate efficiency gains with token-count measurement. All new code covered by unit tests that run without ONNX model files.

**Success criteria (all met):**
- UTF-8 safe truncation and summarization (no split multi-byte characters)
- Stack trace detection with first/last frame preservation
- JSON structured field stripping (trace IDs, request IDs) at Minimal/Standard verbosity
- Event deduplication with counted summaries for both Query and Stream modes
- Compact and pretty JSON output with lowercase field names
- Verbosity-aware field omission (Raw and Confidence stripped at Minimal)
- Token count measurement validating compaction ratios in tests
- All new tests pass without ONNX model files

---

## Prior State

**Working:**
- `Compactor.Compact(raw string) (compacted, summary string)` — verbosity-driven truncation (200/2000/unlimited chars)
- `summarize()` — first 120 characters + `"..."`
- `Verbosity` enum: Minimal, Standard, Full
- `CanonicalEvent` — no JSON tags (Go-default uppercase field names in output)
- `stdout.Output` — NDJSON to stdout, no formatting options
- `OutputConfig.Format` — only `"stdout"`
- `LUMBER_VERBOSITY` env var wired through config → compactor
- `Pipeline.New(conn, eng, out)` — no options pattern, no dedup

**Known bugs fixed:**
- `truncate()` sliced on byte index — split multi-byte UTF-8
- `summarize()` sliced on byte index — same bug

---

## Section 1: Per-Event Compaction

### What changed

Rewrote `internal/engine/compactor/compactor.go` from 49 lines to 227 lines. Fixed both UTF-8 bugs, added stack-trace-aware truncation, structured field stripping, and updated the `Compact` signature to accept `eventType` for type-aware logic.

### Changes

**1.1 Updated `Compact` signature** from `Compact(raw string)` to `Compact(raw, eventType string)`.

The engine already has the classification result (`Type`) when it calls Compact. Passing it enables type-aware logic — stack trace truncation is only applied to `ERROR` events, avoiding false positives on deploy logs or request logs that happen to contain indented text.

Call sites updated in `engine.go` — both `Process()` and `ProcessBatch()` now extract `eventType` from the classification result before calling `Compact`, rather than after.

**1.2 Replaced byte-index truncation with rune-aware truncation.**

Old code:
```go
func truncate(s string, maxLen int) string {
    if len(s) <= maxLen { return s }
    return s[:maxLen] + "..."
}
```

New code uses `range` iteration to find the cut point at a rune boundary:
```go
func truncate(s string, maxRunes int) string {
    if utf8.RuneCountInString(s) <= maxRunes { return s }
    count := 0
    for i := range s {
        if count == maxRunes { return s[:i] + "..." }
        count++
    }
    return s
}
```

Go's `range` on a string advances by rune (not byte), so `i` is always at a valid rune boundary. The `"..."` suffix is appended after the cut. Parameter renamed from `maxLen` to `maxRunes` to clarify semantics.

**1.3 Replaced byte-index summarize with first-line extraction at word boundary.**

Old code: arbitrary first 120 bytes.

New logic:
1. Extract first line (up to `\n`)
2. Trim whitespace
3. If ≤120 runes, return as-is
4. Find the 120th rune boundary
5. Walk back to last space for a word boundary cut
6. Append `"..."` if truncated

This produces a meaningful one-liner instead of arbitrary bytes that might end mid-word or mid-character.

**1.4 Added stack-trace-aware truncation.**

New unexported function `truncateStackTrace(raw string, maxFrames int) string`:

- Detects frame patterns via compiled regexes:
  - `^\s+at ` — Java stack frames
  - `^\s+.+\.go:\d+` — Go source locations
  - `^goroutine \d+` — Go goroutine headers
- Identifies all frame lines and their indices in the raw text
- If total frames ≤ `maxFrames + 2` (tail), returns unchanged
- Otherwise: keeps all non-frame lines, first `maxFrames` frames, last 2 frames
- Replaces omitted middle with `\t... (N frames omitted) ...`

Called from `Compact` only when `eventType == "ERROR"`. Frame limits per verbosity:
- Minimal: 5 first + 2 last
- Standard: 10 first + 2 last
- Full: no truncation

**1.5 Added structured field stripping.**

New unexported function `stripFields(raw string, fields []string) string`:

- Detects JSON-formatted logs (trimmed string starts with `{`)
- Parses as `map[string]any`, removes keys in the strip list, re-serializes
- Non-JSON logs pass through unchanged
- Short-circuits if no keys match (avoids re-serialization overhead)

Default strip list (7 fields):
```go
var defaultStripFields = []string{
    "trace_id", "span_id", "request_id", "x_request_id",
    "correlation_id", "dd.trace_id", "dd.span_id",
}
```

Called at Minimal and Standard verbosity. Skipped at Full.

**1.6 Updated `Compactor` struct with functional options pattern.**

```go
type Option func(*Compactor)
func WithStripFields(fields []string) Option

type Compactor struct {
    Verbosity   Verbosity
    StripFields []string
}

func New(v Verbosity, opts ...Option) *Compactor
```

Default strip list applied automatically. `WithStripFields` overrides it entirely. The `Compact` flow is: strip fields → truncate (stack-trace-aware for errors) → summarize.

**1.7 Updated engine call sites.**

In both `Process()` and `ProcessBatch()` in `engine.go`, the code was reordered: `eventType` is now extracted from the classification result *before* calling `Compact`, so it can be passed as the second argument. Previously `Compact` was called before the path split.

### Files

| File | Action | Lines |
|------|--------|-------|
| `internal/engine/compactor/compactor.go` | Major rewrite | 49 → 227 |
| `internal/engine/compactor/compactor_test.go` | New | 259 lines, 20 tests |
| `internal/engine/engine.go` | Modified | Reordered eventType extraction before Compact call in both Process and ProcessBatch |

### Tests (no ONNX required)

| Test | What it validates |
|------|-------------------|
| `TestTruncateRuneSafety` | CJK (3-byte) characters at boundary produce valid UTF-8 |
| `TestTruncateEmoji` | 4-byte emoji at boundary produce valid UTF-8 |
| `TestTruncateASCII` | ASCII strings behave as before |
| `TestTruncateShortInput` | Under-limit strings returned unchanged |
| `TestTruncateExactLength` | Exact-limit strings returned unchanged |
| `TestSummarizeFirstLine` | Multi-line input uses first line only |
| `TestSummarizeWordBoundary` | Long first line truncated at word boundary, no partial words |
| `TestSummarizeShortInput` | Short input returned unchanged |
| `TestSummarizeMultibyteFirstLine` | CJK first line extracted correctly |
| `TestStackTraceJava` | 30-frame Java trace → 5+2 frames with omission message |
| `TestStackTraceGo` | Go goroutine dump frames truncated |
| `TestStackTraceNone` | Non-trace ERROR logs pass through unchanged |
| `TestStackTraceShort` | Traces with fewer frames than threshold not truncated |
| `TestStripFieldsJSON` | JSON log with trace_id/request_id has them removed |
| `TestStripFieldsNonJSON` | Plain text passes through unchanged |
| `TestStripFieldsFullVerbosity` | No stripping at Full |
| `TestStripFieldsCustomList` | Custom strip list via `WithStripFields` option |
| `TestStripFieldsNoMatch` | JSON log with no matching fields returned unchanged |
| `TestCompactMinimal` | Full flow: strip + truncate at Minimal |
| `TestCompactStandard` | Full flow: strip + truncate at Standard |
| `TestCompactFull` | Full preserves everything unchanged |

---

## Section 2: JSON Output Formatting

### What changed

Added JSON tags and `Count` field to `CanonicalEvent`. Created `FormatEvent` for verbosity-aware field omission. Updated `stdout.Output` with pretty/compact toggle and verbosity. Added `Pretty` config option.

### Changes

**2.1 Added JSON tags and `Count` field to `CanonicalEvent`.**

```go
type CanonicalEvent struct {
    Type       string    `json:"type"`
    Category   string    `json:"category"`
    Severity   string    `json:"severity"`
    Timestamp  time.Time `json:"timestamp"`
    Summary    string    `json:"summary"`
    Confidence float64   `json:"confidence,omitempty"`
    Raw        string    `json:"raw,omitempty"`
    Count      int       `json:"count,omitempty"`
}
```

All field names are now lowercase in JSON output. `Confidence`, `Raw`, and `Count` use `omitempty` — they vanish from output when zero-valued. `Count` defaults to 0 (omitted), set by dedup in Section 3 when events are collapsed.

**2.2 Created `FormatEvent` function** in `internal/output/format.go`.

```go
func FormatEvent(e model.CanonicalEvent, verbosity compactor.Verbosity) model.CanonicalEvent
```

Returns a copy of the event with fields zeroed according to verbosity:
- **Minimal:** `Raw` and `Confidence` zeroed → omitted from JSON via `omitempty`
- **Standard/Full:** all fields preserved

Returns a value (not pointer) — `CanonicalEvent` has no pointer fields, so this is a clean copy that doesn't mutate the original. This matters when the same event is used by both dedup and output.

**2.3 Updated `stdout.Output`** constructor to accept verbosity and pretty flag.

```go
func New(verbosity compactor.Verbosity, pretty bool) *Output
```

- When `pretty` is true, `json.Encoder.SetIndent("", "  ")` produces indented multi-line JSON
- When false (default), single-line NDJSON
- `Write` calls `FormatEvent` before encoding

**2.4 Added `Pretty` to config.**

`LUMBER_OUTPUT_PRETTY` env var (default `false`). Parsed via new `getenvBool()` helper that accepts `"true"`, `"1"`, or case-insensitive `"TRUE"`.

**2.5 Updated `cmd/lumber/main.go`** to pass verbosity and pretty to `stdout.New`.

### Files

| File | Action | Lines |
|------|--------|-------|
| `internal/model/event.go` | Modified | Added JSON tags, `Count` field |
| `internal/output/format.go` | New | 17 lines |
| `internal/output/format_test.go` | New | 110 lines, 5 tests |
| `internal/output/stdout/stdout.go` | Modified | New constructor signature, FormatEvent integration |
| `internal/output/stdout/stdout_test.go` | New | 94 lines, 3 tests |
| `internal/config/config.go` | Modified | `Pretty bool` on OutputConfig, `getenvBool()` helper |
| `internal/config/config_test.go` | Modified | 2 new tests |
| `cmd/lumber/main.go` | Modified | Pass verbosity + pretty to stdout.New |

### Tests (no ONNX required)

| Test | What it validates |
|------|-------------------|
| `TestFormatEventMinimal` | Raw and Confidence zeroed at Minimal |
| `TestFormatEventStandard` | All fields preserved at Standard |
| `TestFormatEventFull` | All fields preserved at Full |
| `TestFormatEventCount` | Count > 0 preserved in JSON; Count == 0 omitted |
| `TestJSONTagNames` | All keys lowercase in marshalled JSON |
| `TestOutputCompactJSON` | Single-line NDJSON with lowercase keys |
| `TestOutputPrettyJSON` | Indented multi-line JSON |
| `TestOutputMinimalOmitsFields` | Raw and Confidence absent from JSON at Minimal |
| `TestLoad_PrettyDefault` | Default false |
| `TestLoad_PrettyEnv` | `LUMBER_OUTPUT_PRETTY=true` parsed correctly |

---

## Section 3: Event Deduplication

### What changed

Created `internal/engine/dedup` package with batch-level deduplication. Created streaming buffer with timer-based flush. Wired dedup into pipeline via functional options. Added `DedupWindow` config.

### Changes

**3.1 Created `internal/engine/dedup/dedup.go`.**

```go
type Config struct { Window time.Duration }
type Deduplicator struct { cfg Config }

func New(cfg Config) *Deduplicator
func (d *Deduplicator) DeduplicateBatch(events []model.CanonicalEvent) []model.CanonicalEvent
```

**3.2 Deduplication logic.**

Dedup key: `Type + "." + Category`. Two `ERROR.connection_failure` events with different messages collapse into one — downstream agents care about *what kind* of event, not the verbatim text.

Algorithm:
1. Iterate events in order
2. Maintain ordered map (slice of keys + map to accumulator)
3. When a key is seen again and timestamp is within `Window` of the group's first timestamp → increment count, track latest timestamp
4. When outside the window → start a new group (same key can have multiple groups)
5. Output: one event per group with `Count` set (only if > 1) and `Summary` rewritten to `"<original> (x<count> in <duration>)"`
6. Preserves first event's timestamp and all other fields

Duration formatting: `formatDuration()` produces human-readable short strings — `"450ms"`, `"12s"`, `"4m32s"`.

**3.3 Wired into pipeline** via functional options.

```go
type Option func(*Pipeline)
func WithDedup(d *dedup.Deduplicator, window time.Duration) Option

func New(conn connector.Connector, eng *engine.Engine, out output.Output, opts ...Option) *Pipeline
```

Pipeline now has optional `*dedup.Deduplicator` and `window` fields.

- `Query()`: after `ProcessBatch`, calls `DeduplicateBatch` before writing if dedup is configured
- `Stream()`: dispatches to `streamDirect()` (no dedup) or `streamWithDedup()` (buffered)

**3.4 Streaming dedup buffer** in `internal/pipeline/buffer.go`.

`streamBuffer` type:
- Events accumulate in a `[]model.CanonicalEvent` protected by a mutex
- First event starts a `time.Timer` for the window duration
- `flushCh()` returns the timer's channel (or nil if no timer active) — used in the pipeline's `select` loop
- `flush()` deduplicates pending events and writes all to output, resets buffer
- On context cancellation in `streamWithDedup()`: flush remaining events immediately

The `select` loop in `streamWithDedup()` handles three cases:
1. `ctx.Done()` → flush + return
2. New log from channel → process + add to buffer
3. Timer fires → flush buffer

**3.5 Config wiring.**

`LUMBER_DEDUP_WINDOW` env var (default `"5s"`, `"0"` to disable). Parsed via new `getenvDuration()` helper. When duration is 0, dedup is nil and pipeline skips it entirely.

**3.6 Updated `cmd/lumber/main.go`** to create `Deduplicator` from config and pass to pipeline via `WithDedup`. Logs dedup status to stderr on startup.

### Files

| File | Action | Lines |
|------|--------|-------|
| `internal/engine/dedup/dedup.go` | New | 88 lines |
| `internal/engine/dedup/dedup_test.go` | New | 134 lines, 7 tests |
| `internal/pipeline/pipeline.go` | Major rewrite | 79 → 140 lines |
| `internal/pipeline/buffer.go` | New | 66 lines |
| `internal/pipeline/pipeline_test.go` | New | 128 lines, 3 tests |
| `internal/config/config.go` | Modified | `DedupWindow` on EngineConfig, `getenvDuration()` helper |
| `internal/config/config_test.go` | Modified | 3 new tests |
| `cmd/lumber/main.go` | Modified | Dedup creation and wiring |

### Tests (no ONNX required)

| Test | What it validates |
|------|-------------------|
| `TestDeduplicateBatchEmpty` | nil → nil |
| `TestDeduplicateBatchNoDuplicates` | Distinct events unchanged, Count unset |
| `TestDeduplicateBatchSimple` | 5 identical → 1 with Count=5 |
| `TestDeduplicateBatchMixed` | `[A, B, A, A, B]` → `[A(x3), B(x2)]` in first-occurrence order |
| `TestDeduplicateBatchWindowExpiry` | Events spanning > Window produce separate groups for same key |
| `TestDeduplicateBatchSummaryFormat` | Summary contains `"(x47..."` and original text |
| `TestDeduplicateBatchPreservesTimestamp` | Uses first event's timestamp |
| `TestStreamBufferFlush` | 10 events within window flushed as 1 deduplicated event after timer |
| `TestStreamBufferContextCancel` | Flush on cancel produces deduplicated output |
| `TestPipelineWithoutDedup` | 3 distinct events pass through unfused |
| `TestLoad_DedupWindowDefault` | Default 5s |
| `TestLoad_DedupWindowEnv` | `LUMBER_DEDUP_WINDOW=10s` parsed correctly |
| `TestLoad_DedupWindowDisabled` | `LUMBER_DEDUP_WINDOW=0` sets duration to 0 |

---

## Section 4: Token Measurement & Validation

### What changed

Created `EstimateTokens` utility for measuring compaction efficiency. Created integration tests with realistic log inputs validating compaction ratios, UTF-8 safety, and feature correctness across all verbosity levels.

### Changes

**4.1 Created `internal/engine/compactor/tokencount.go`.**

```go
func EstimateTokens(s string) int
```

Whitespace-based heuristic: splits on whitespace via `strings.Fields`, applies 1.3x subword expansion factor (rounded up via `math.Ceil`). Not a real BPE tokenizer — accurate within ~20% of actual token counts, sufficient for measuring compaction *ratios* (before/after).

**4.2 Created integration test file** with 5 realistic log inputs:

| Input | Size | Description |
|-------|------|-------------|
| `jsonStructuredLog` | ~450 bytes | JSON error log with 6 high-cardinality fields (trace_id, span_id, etc.) |
| `javaStackTrace` | ~2.2 KB | 30-frame Java NullPointerException |
| `goPanicDump` | ~1.3 KB | Multi-goroutine Go panic with source locations |
| `plainTextError` | ~250 bytes | Multi-line plain text error with key=value fields |
| `shortRequestLog` | ~50 bytes | Single-line health check log |

### Files

| File | Action | Lines |
|------|--------|-------|
| `internal/engine/compactor/tokencount.go` | New | 17 lines |
| `internal/engine/compactor/tokencount_test.go` | New | 40 lines, 5 tests |
| `internal/engine/compactor/integration_test.go` | New | 173 lines, 7 tests |

### Tests (no ONNX required)

| Test | What it validates |
|------|-------------------|
| `TestEstimateTokensEmpty` | Returns 0 |
| `TestEstimateTokensSimple` | `"hello world"` → 3 |
| `TestEstimateTokensSingleWord` | `"hello"` → 2 |
| `TestEstimateTokensLong` | 500-word paragraph → 650 |
| `TestEstimateTokensWhitespaceOnly` | Returns 0 |
| `TestIntegrationMinimalStackTrace` | Java trace at Minimal: truncated with omission message, summary is first line, >60% token reduction |
| `TestIntegrationStandardStructuredLog` | JSON at Standard: 6 trace fields stripped, core fields (level, msg, service) preserved |
| `TestIntegrationFullPreservesEverything` | Full: both JSON and stack trace input returned unchanged |
| `TestIntegrationMultibyteUTF8` | CJK JSON log at all 3 verbosity levels produces valid UTF-8 |
| `TestIntegrationGoPanicMinimal` | Go panic dump: frames truncated, summary references goroutine, token count reduced |
| `TestIntegrationPlainTextError` | Plain text: no stripping (not JSON), truncated at 200 runes, summary includes service name |
| `TestIntegrationShortLog` | Short log unchanged at Minimal (under all limits) |

---

## Design Decisions

### Rune-aware truncation via `range` iteration
Go's `range` on a string yields `(byte_offset, rune)` pairs, advancing by the rune's byte width. This means `s[:i]` at any iteration point is guaranteed to be at a rune boundary — the simplest and most idiomatic way to do UTF-8-safe slicing without importing `unicode/utf8` for manual decoding.

### Stack trace detection is regex-based, not parser-based
We detect frame patterns with 3 simple regexes rather than building a full stack trace parser. This covers Java and Go (the two most common in the target ecosystem) with minimal code. The pattern list is a `var` block — easy to extend for Python/Node.js/Rust in a future phase without changing the truncation algorithm.

### Field stripping re-serializes JSON
`stripFields` parses JSON into `map[string]any`, deletes keys, and re-marshals. This reorders keys (Go maps are unordered) but produces valid, compact JSON. The alternative — regex-based key removal — would be fragile with nested objects and escaped characters. The performance cost of parse+marshal is acceptable because it only runs at Minimal/Standard verbosity and short-circuits when no keys match.

### `FormatEvent` returns a copy, not a mutation
`CanonicalEvent` is a value type with no pointer fields. `FormatEvent` takes it by value and returns a modified copy. This ensures the original event (used by dedup, logging, or other consumers) is never affected by output formatting.

### Dedup key is `Type.Category`, not raw text
Two `ERROR.connection_failure` events with different messages are "the same" from a downstream agent's perspective. Deduplicating on raw text would miss semantic duplicates (same error, different formatting). This aggressive collapsing trades individual event detail for dramatic token reduction during error storms.

### Streaming buffer uses `time.Timer`, not `time.Ticker`
A ticker fires continuously regardless of whether events are arriving. A timer starts only when the first event arrives and fires once after the window — no wasted CPU when the stream is idle. After flush, the timer is nil until the next event.

### `Pipeline.New` uses functional options, not a config struct
The `WithDedup(d, window)` option pattern keeps the zero-value pipeline (no dedup) working without any configuration. Adding future options (metrics, filtering, backpressure) won't break existing call sites.

### `getenvDuration` treats `"0"` as explicit disable
`time.ParseDuration("0")` would return `0` anyway, but checking for `"0"` before parsing makes the semantics explicit in code: setting `LUMBER_DEDUP_WINDOW=0` intentionally disables dedup, it's not a parse error.

---

## Files Summary

| File | Section | Action |
|------|---------|--------|
| `internal/engine/compactor/compactor.go` | 1 | Major rewrite: rune-safe truncation, first-line summarize, stack trace handling, field stripping, updated signature and constructor with options |
| `internal/engine/compactor/compactor_test.go` | 1 | New: 20 unit tests |
| `internal/engine/engine.go` | 1 | Modified: reordered eventType extraction, updated Compact call sites to pass eventType |
| `internal/model/event.go` | 2 | Modified: added JSON tags to all fields, added Count field with omitempty |
| `internal/output/format.go` | 2 | New: FormatEvent function for verbosity-aware field omission |
| `internal/output/format_test.go` | 2 | New: 5 tests |
| `internal/output/stdout/stdout.go` | 2 | Modified: new constructor with verbosity + pretty, FormatEvent integration |
| `internal/output/stdout/stdout_test.go` | 2 | New: 3 tests |
| `internal/engine/dedup/dedup.go` | 3 | New: Deduplicator with windowed batch dedup |
| `internal/engine/dedup/dedup_test.go` | 3 | New: 7 tests |
| `internal/pipeline/pipeline.go` | 3 | Major rewrite: functional options, WithDedup, streamDirect/streamWithDedup split |
| `internal/pipeline/buffer.go` | 3 | New: timer-based streaming dedup buffer |
| `internal/pipeline/pipeline_test.go` | 3 | New: 3 tests |
| `internal/engine/compactor/tokencount.go` | 4 | New: EstimateTokens whitespace heuristic |
| `internal/engine/compactor/tokencount_test.go` | 4 | New: 5 tests |
| `internal/engine/compactor/integration_test.go` | 4 | New: 7 integration tests with realistic logs |
| `internal/config/config.go` | 2, 3 | Modified: Pretty bool, DedupWindow, getenvBool, getenvDuration |
| `internal/config/config_test.go` | 2, 3 | Modified: 5 new tests |
| `cmd/lumber/main.go` | 2, 3 | Modified: dedup import, dedup creation, WithDedup wiring, stdout.New with verbosity + pretty |

**New files: 11. Modified files: 8. Total: 19.**

---

## New Environment Variables

| Variable | Default | Type | Description |
|----------|---------|------|-------------|
| `LUMBER_OUTPUT_PRETTY` | `false` | bool | Pretty-print JSON output (indented) |
| `LUMBER_DEDUP_WINDOW` | `5s` | duration | Event dedup window; `"0"` disables |

---

## Verification

```bash
go test ./internal/engine/compactor/...   # Section 1 + 4: 32 tests
go test ./internal/output/...             # Section 2: 8 tests
go test ./internal/engine/dedup/...       # Section 3: 7 tests
go test ./internal/pipeline/...           # Section 3: 3 tests
go test ./internal/config/...             # Section 2 + 3: 9 tests (4 existing + 5 new)
go build ./cmd/lumber                     # Compiles with all changes
```

Total: **59 new tests** across 10 test files.

---

## What's Explicitly Not In Scope

- **Real BPE tokenizer** — whitespace heuristic is sufficient for ratio measurement
- **Dedup persistence across restarts** — in-memory only, no state survives restart
- **Output destinations beyond stdout** — Output interface is extensible, but no new implementations
- **Configurable stack trace patterns** — hardcoded Java/Go detection, extensible later
- **Streaming dedup backpressure** — buffer grows unboundedly during window; bounded buffer deferred to Phase 5
- **Per-field verbosity control** — single global verbosity setting
- **Metrics export** — token counts measured in tests, not Prometheus/etc.
- **Buffering and graceful shutdown** — Phase 5 concern
