# Compactor & Output Blueprint

## Overview

Lumber's compactor and output layer sits between the classification engine and the consumer. It takes a classified `CanonicalEvent` — already tagged with type, category, and severity — and makes it production-ready: token-efficient, safe to serialize, and collapsed when repetitive.

The layer solves four problems:

1. **Token waste** — raw logs carry verbose noise (30-frame stack traces, high-cardinality trace IDs, redundant metadata). The compactor strips, truncates, and summarizes so downstream consumers — especially LLM agents paying per-token — get the signal without the bulk.
2. **UTF-8 safety** — naive byte-index truncation can split multi-byte characters, producing invalid UTF-8 in output. All truncation operates on rune boundaries.
3. **Event storms** — when a service fails, the same error repeats hundreds of times. The deduplicator collapses semantically identical events into counted summaries, turning 500 `ERROR.connection_failure` events into one with `count: 500`.
4. **Output formatting** — JSON field names, verbosity-aware field omission, and pretty-print toggling make the output consumable by both machines and humans.

The compactor is type-aware (stack trace truncation only applies to `ERROR` events), verbosity-driven (three levels control how aggressively logs are compacted), and deterministic (same input + same config = same output).

---

## Compaction Pipeline

Compaction runs per-event inside the engine's `Process` and `ProcessBatch` methods, after classification but before output. The flow has three stages, applied in order:

```
Raw log text + event type
    │
    ▼
┌─────────────────────────────────────────────────────┐
│ 1. STRIP FIELDS                                      │
│                                                      │
│   JSON logs only. Remove high-cardinality fields     │
│   (trace_id, span_id, request_id, etc.)              │
│   Applied at Minimal and Standard. Skipped at Full.  │
└──────────────────────┬───────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│ 2. TRUNCATE                                          │
│                                                      │
│   a. Stack trace truncation (ERROR events only)      │
│      Keep first N + last 2 frames, omit middle       │
│   b. Character truncation (if stack trace truncation │
│      didn't fire): rune-safe limit per verbosity     │
│                                                      │
│   Minimal: 5 first + 2 last frames, or 200 runes    │
│   Standard: 10 first + 2 last frames, or 2000 runes │
│   Full: no truncation                                │
└──────────────────────┬───────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────┐
│ 3. SUMMARIZE                                         │
│                                                      │
│   Extract first line, trim whitespace.               │
│   If > 120 runes, cut at word boundary + "..."       │
│   Applied at all verbosity levels.                   │
└──────────────────────┬───────────────────────────────┘
                       │
                       ▼
    (compacted string, summary string)
```

The compactor returns two strings: the compacted raw text (for the `Raw` field) and a one-line summary (for the `Summary` field). Both are attached to the `CanonicalEvent` by the engine.

---

## Compactor

### Constructor

```go
type Compactor struct {
    Verbosity   Verbosity
    StripFields []string
}

type Option func(*Compactor)
func WithStripFields(fields []string) Option

func New(v Verbosity, opts ...Option) *Compactor
```

The functional options pattern keeps the default (7-field strip list) working out of the box while allowing override. `WithStripFields` replaces the entire strip list — it doesn't append.

### Verbosity levels

```go
type Verbosity int
const (
    Minimal  Verbosity = iota  // Aggressive: 200 runes, 5 stack frames
    Standard                    // Balanced: 2000 runes, 10 stack frames
    Full                        // Preserve everything
)
```

Configured via `LUMBER_VERBOSITY` env var (default `"standard"`).

### Compact signature

```go
func (c *Compactor) Compact(raw, eventType string) (compacted, summary string)
```

The `eventType` parameter enables type-aware logic. Stack trace truncation only applies when `eventType == "ERROR"` — a deploy log that happens to contain indented text won't have its build output mangled.

The engine extracts `eventType` from the classification result *before* calling `Compact`, ensuring the type is available without requiring the compactor to understand classification.

---

## Field Stripping

The first stage removes high-cardinality structured fields that waste tokens without carrying actionable information for downstream consumers.

### How it works

```go
func stripFields(raw string, fields []string) string
```

1. Check if the trimmed string starts with `{` (JSON detection)
2. Parse as `map[string]any`
3. Delete keys in the strip list
4. If no keys matched, return unchanged (avoids re-serialization overhead)
5. Re-marshal to compact JSON

Non-JSON logs pass through unchanged. The function never errors — parse failures simply return the original text.

### Default strip list

```go
var defaultStripFields = []string{
    "trace_id", "span_id", "request_id", "x_request_id",
    "correlation_id", "dd.trace_id", "dd.span_id",
}
```

These are observability-infrastructure fields: useful for distributed tracing tools, noise for LLM agents and dashboards. A JSON log with 6 trace fields drops ~120 tokens after stripping.

Stripping runs at Minimal and Standard verbosity. At Full, all fields are preserved — the assumption is that Full verbosity consumers want the complete original log.

---

## Stack Trace Truncation

The second stage handles the most common source of token bloat in error logs: multi-frame stack traces. A 30-frame Java exception or a Go panic dump with 4 goroutines can be 2-3KB of repetitive text where only the top and bottom frames carry signal.

### How it works

```go
func truncateStackTrace(raw string, maxFrames int) string
```

1. **Detect frames** via compiled regexes:
   - `^\s+at ` — Java/JavaScript stack frames
   - `^\s+.+\.go:\d+` — Go source file locations
   - `^goroutine \d+` — Go goroutine headers

2. **Count total frames.** If total frames ≤ `maxFrames + 2` (the tail allowance), return unchanged — no point truncating a short trace.

3. **Range-based cut.** Everything between the last kept first-frame and the first kept last-frame is replaced wholesale with a single `\t... (N frames omitted) ...` message.

4. **Return.** The result preserves all non-frame lines (exception messages, "Caused by" headers, goroutine state lines) while replacing only the redundant middle frames.

### Frame limits per verbosity

| Verbosity | First frames | Last frames | Example: 30-frame Java trace |
|-----------|-------------|-------------|------------------------------|
| Minimal | 5 | 2 | 7 frames + omission message |
| Standard | 10 | 2 | 12 frames + omission message |
| Full | No truncation | — | All 30 frames preserved |

### Interaction with character truncation

Stack trace truncation and character truncation serve the same purpose — reducing token count. When stack trace truncation fires (returns a different string), character truncation is skipped. This prevents a truncated-but-still-long trace (e.g., 7 frames at ~660 chars) from having its `... (N frames omitted) ...` message clipped by the 200-rune character limit.

---

## Character Truncation

When stack trace truncation doesn't apply (non-ERROR events, or ERROR events without detectable stack frames), character truncation provides a hard limit on output size.

### Rune-safe truncation

```go
func truncate(s string, maxRunes int) string
```

Uses Go's `range` iteration over the string, which advances by rune (not byte). The loop variable `i` is always a valid rune boundary, so `s[:i]` never splits a multi-byte character.

```go
count := 0
for i := range s {
    if count == maxRunes { return s[:i] + "..." }
    count++
}
```

For a CJK log line where every character is 3 bytes, byte-index truncation at position 200 would land inside a character. Rune-index truncation at rune 200 lands at byte position 600 — always valid UTF-8.

### Limits

| Verbosity | Max runes | Suffix |
|-----------|-----------|--------|
| Minimal | 200 | `"..."` |
| Standard | 2,000 | `"..."` |
| Full | Unlimited | — |

---

## Summarization

The final stage produces a one-line summary for every event, regardless of verbosity.

```go
func summarize(raw string) string
```

1. Extract first line (up to `\n`)
2. Trim whitespace
3. If ≤ 120 runes, return as-is
4. Find the 120th rune boundary
5. Walk back to the last space for a word-boundary cut
6. Append `"..."` if truncated

This produces a meaningful one-liner instead of arbitrary bytes that might end mid-word. A 30-frame Java exception's summary is its exception message: `"NullPointerException: Cannot invoke method on null object"`.

---

## Event Deduplication

After the engine classifies and compacts a batch of events, the deduplicator collapses semantically identical events into counted summaries. This is the batch-level token optimization — during an error storm, 500 connection failures become one event with `count: 500`.

### Dedup key

```
Type + "." + Category
```

Two `ERROR.connection_failure` events with different messages are "the same" from a downstream agent's perspective. The agent cares about *what kind* of event occurred and *how many times*, not the verbatim text of each instance. This aggressive keying trades individual event detail for dramatic token reduction.

### Algorithm

```go
type Deduplicator struct { cfg Config }

func (d *Deduplicator) DeduplicateBatch(events []CanonicalEvent) []CanonicalEvent
```

1. Iterate events in order
2. Maintain an ordered map (slice of keys + map to accumulator) preserving first-occurrence order
3. When a key is seen again and the timestamp is within `Window` of the group's first timestamp → increment count, track latest timestamp
4. When outside the window → start a new group (same key can have multiple groups)
5. Output: one event per group with `Count` set (only if > 1) and `Summary` rewritten to `"<original> (x<count> in <duration>)"`
6. First event's timestamp and all other fields are preserved

### Duration formatting

`formatDuration()` produces human-readable short strings: `"450ms"`, `"12s"`, `"4m32s"`. Used in the summary suffix so consumers see at a glance how compressed the window is.

### Configuration

```
LUMBER_DEDUP_WINDOW=5s     # default, env var override
LUMBER_DEDUP_WINDOW=0      # explicitly disables dedup
```

When the window is 0, dedup is nil and the pipeline skips it entirely — no allocation, no overhead.

---

## Streaming Dedup Buffer

Dedup in `Query()` mode is straightforward — batch is fully collected, then deduplicated. `Stream()` mode is continuous and needs a windowed buffer.

### Buffer design

`streamBuffer` in `internal/pipeline/buffer.go`:

- Events accumulate in a `[]model.CanonicalEvent` protected by a mutex
- First event starts a `time.Timer` for the window duration
- `flushCh()` returns the timer's channel (or `nil` if no timer active) — used in the pipeline's `select` loop
- `flush()` deduplicates pending events and writes all to output, then resets the buffer
- On context cancellation: flush remaining events immediately (no data loss)

### Select loop

The `streamWithDedup()` function handles three cases:

```go
select {
case <-ctx.Done():    // Flush remaining + return
case log := <-ch:     // Process + add to buffer
case <-buf.flushCh(): // Timer fired → flush buffer
}
```

When no events are arriving, no timer is active — zero CPU waste. The timer only starts when the first event of a new window arrives.

---

## Output Formatting

### CanonicalEvent schema

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

All field names are lowercase in JSON output via struct tags. `Confidence`, `Raw`, and `Count` use `omitempty` — they vanish from output when zero-valued. `Count` is set by dedup when events are collapsed (> 1); singleton events omit it entirely.

### Verbosity-aware field omission

```go
func FormatEvent(e model.CanonicalEvent, verbosity compactor.Verbosity) model.CanonicalEvent
```

Returns a copy of the event with fields zeroed according to verbosity:

| Verbosity | Fields omitted |
|-----------|---------------|
| Minimal | `Raw`, `Confidence` |
| Standard | None |
| Full | None |

Returns a value (not pointer) — `CanonicalEvent` has no pointer fields, so this is a clean copy that doesn't mutate the original. This matters when the same event is used by both dedup and output.

### stdout output

```go
func New(verbosity compactor.Verbosity, pretty bool) *Output
```

- **Compact mode** (default): single-line NDJSON, one event per line
- **Pretty mode** (`LUMBER_OUTPUT_PRETTY=true`): indented multi-line JSON via `json.Encoder.SetIndent("", "  ")`

`Write` calls `FormatEvent` before encoding, so field omission is always applied.

### Example output

**Minimal, compact:**
```json
{"type":"ERROR","category":"connection_failure","severity":"error","timestamp":"2026-02-23T10:30:00Z","summary":"connection refused (host=db-primary)","count":47}
```

**Standard, pretty:**
```json
{
  "type": "ERROR",
  "category": "connection_failure",
  "severity": "error",
  "timestamp": "2026-02-23T10:30:00Z",
  "summary": "connection refused (host=db-primary)",
  "confidence": 0.87,
  "raw": "ERROR [2026-02-23 10:30:00] UserService — connection refused (host=db-primary, port=5432)"
}
```

---

## Pipeline Integration

The pipeline (`internal/pipeline/pipeline.go`) wires connectors, engine, and output together. Dedup is injected via functional options:

```go
type Option func(*Pipeline)
func WithDedup(d *dedup.Deduplicator, window time.Duration) Option

func New(conn connector.Connector, eng *engine.Engine, out output.Output, opts ...Option) *Pipeline
```

The zero-value pipeline (no options) works without dedup — `Query()` passes events straight to output, `Stream()` uses `streamDirect()`.

When dedup is configured:
- `Query()` calls `DeduplicateBatch` after `ProcessBatch`, before writing
- `Stream()` dispatches to `streamWithDedup()` with the timer-based buffer

---

## Token Efficiency

### Measurement

```go
func EstimateTokens(s string) int
```

Whitespace-based heuristic: splits on whitespace via `strings.Fields`, applies 1.3× subword expansion factor (rounded up via `math.Ceil`). Not a real BPE tokenizer — accurate within ~20% of actual token counts, sufficient for measuring compaction *ratios* (before/after).

### Validated compaction ratios

Measured against realistic log inputs in integration tests:

| Input | Size | Minimal reduction | What's removed |
|-------|------|-------------------|----------------|
| 30-frame Java stack trace | ~2.2 KB | > 60% | Middle frames replaced by omission message |
| JSON error log with 6 trace fields | ~450 bytes | ~30% | trace_id, span_id, request_id, etc. |
| Multi-goroutine Go panic | ~1.3 KB | > 40% | Goroutine frames truncated |
| Plain text error (250 bytes) | ~250 bytes | Truncated to 200 runes | Character limit |
| Short health check log (50 bytes) | ~50 bytes | Unchanged | Under all limits |

At Minimal verbosity with dedup, an error storm of 500 identical connection failures collapses from ~125KB of raw text to a single ~200-byte JSON event — a >99% reduction.

---

## Test Strategy

All compactor and output tests run without ONNX model files — they exercise pure Go logic only.

### Compactor unit tests (20 tests)

| Test group | Count | What's validated |
|------------|-------|-----------------|
| `truncate` | 5 | CJK boundary safety, emoji boundary safety, ASCII, short input, exact length |
| `summarize` | 4 | First line extraction, word boundary cut, short input, multibyte first line |
| `truncateStackTrace` | 4 | Java 30-frame trace, Go goroutine dump, non-trace passthrough, short trace passthrough |
| `stripFields` | 5 | JSON stripping, non-JSON passthrough, Full verbosity bypass, custom list, no-match passthrough |
| `Compact` (integration) | 3 | Full flow at each verbosity level |

### Output tests (8 tests)

| Test | What's validated |
|------|-----------------|
| `TestFormatEventMinimal` | Raw and Confidence zeroed |
| `TestFormatEventStandard` | All fields preserved |
| `TestFormatEventFull` | All fields preserved |
| `TestFormatEventCount` | Count > 0 preserved; Count == 0 omitted |
| `TestJSONTagNames` | All keys lowercase in JSON |
| `TestOutputCompactJSON` | Single-line NDJSON |
| `TestOutputPrettyJSON` | Indented multi-line JSON |
| `TestOutputMinimalOmitsFields` | Raw and Confidence absent from JSON at Minimal |

### Dedup tests (7 tests)

| Test | What's validated |
|------|-----------------|
| `TestDeduplicateBatchEmpty` | nil → nil |
| `TestDeduplicateBatchNoDuplicates` | Distinct events unchanged, Count unset |
| `TestDeduplicateBatchSimple` | 5 identical → 1 with Count=5 |
| `TestDeduplicateBatchMixed` | `[A, B, A, A, B]` → `[A(x3), B(x2)]` in first-occurrence order |
| `TestDeduplicateBatchWindowExpiry` | Events spanning > Window produce separate groups |
| `TestDeduplicateBatchSummaryFormat` | Summary contains count and original text |
| `TestDeduplicateBatchPreservesTimestamp` | Uses first event's timestamp |

### Pipeline dedup tests (3 tests)

| Test | What's validated |
|------|-----------------|
| `TestStreamBufferFlush` | 10 events within window flushed as 1 after timer |
| `TestStreamBufferContextCancel` | Flush on cancel produces deduplicated output |
| `TestPipelineWithoutDedup` | 3 distinct events pass through unfused |

### Token measurement tests (12 tests)

| Test group | Count | What's validated |
|------------|-------|-----------------|
| `EstimateTokens` | 5 | Empty, simple, single word, long paragraph, whitespace-only |
| Integration | 7 | Stack trace reduction, field stripping, Full preservation, UTF-8 safety, Go panic, plain text, short log |

### Config tests (5 tests)

| Test | What's validated |
|------|-----------------|
| `TestLoad_PrettyDefault` | Default false |
| `TestLoad_PrettyEnv` | `LUMBER_OUTPUT_PRETTY=true` parsed correctly |
| `TestLoad_DedupWindowDefault` | Default 5s |
| `TestLoad_DedupWindowEnv` | `LUMBER_DEDUP_WINDOW=10s` parsed correctly |
| `TestLoad_DedupWindowDisabled` | `LUMBER_DEDUP_WINDOW=0` sets duration to 0 |

**Total: 59 tests** across 10 test files.

---

## File Layout

```
internal/engine/compactor/
├── compactor.go              Per-event compaction: strip → truncate → summarize
├── compactor_test.go         20 unit tests
├── tokencount.go             EstimateTokens whitespace heuristic
├── tokencount_test.go        5 tests
└── integration_test.go       7 integration tests with realistic logs

internal/engine/dedup/
├── dedup.go                  Batch deduplication with windowed grouping
└── dedup_test.go             7 tests

internal/output/
├── output.go                 Output interface definition
├── format.go                 FormatEvent: verbosity-aware field omission
├── format_test.go            5 tests
└── stdout/
    ├── stdout.go             NDJSON/pretty stdout writer with FormatEvent
    └── stdout_test.go        3 tests

internal/pipeline/
├── pipeline.go               Pipeline orchestration with functional options, WithDedup
├── buffer.go                 Timer-based streaming dedup buffer
└── pipeline_test.go          3 pipeline dedup tests

internal/model/
└── event.go                  CanonicalEvent with JSON tags, Count field, omitempty

internal/config/
├── config.go                 Pretty, DedupWindow, getenvBool, getenvDuration
└── config_test.go            5 new tests (10 total with prior phases)

cmd/lumber/
└── main.go                   Dedup creation, WithDedup wiring, stdout.New with verbosity + pretty
```

---

## Key Constants

| Constant | Value | Location |
|----------|-------|----------|
| Minimal character limit | 200 runes | `compactor.go` |
| Standard character limit | 2,000 runes | `compactor.go` |
| Summary max length | 120 runes | `compactor.go:summarize` |
| Minimal stack frames (first) | 5 | `compactor.go` |
| Standard stack frames (first) | 10 | `compactor.go` |
| Stack frames (last, all levels) | 2 | `compactor.go` |
| Default strip fields | 7 keys | `compactor.go:defaultStripFields` |
| Default dedup window | 5s | `config.go` |
| Token estimate expansion factor | 1.3× | `tokencount.go` |
| Default pretty output | false | `config.go` |

---

## Environment Variables

| Variable | Default | Type | Description |
|----------|---------|------|-------------|
| `LUMBER_VERBOSITY` | `"standard"` | string | Compaction aggressiveness: `minimal`, `standard`, `full` |
| `LUMBER_OUTPUT_PRETTY` | `false` | bool | Pretty-print JSON output (indented) |
| `LUMBER_DEDUP_WINDOW` | `5s` | duration | Event dedup window; `"0"` disables |

---

## Design Decisions

**Descriptions are the dominant lever — compaction is secondary.** The taxonomy description tuning (Phase 2) determines *what category* a log gets. The compactor determines *how much of it* the consumer sees. These are independent concerns. A perfectly classified event that carries 2KB of stack trace is still wasteful. The compactor exists to close the gap between "correctly classified" and "token-efficient."

**Stack trace detection is regex-based, not parser-based.** Three simple regexes cover Java and Go stack frames — the two most common in the target ecosystem. The pattern list is a `var` block, trivially extensible for Python/Node.js/Rust without changing the truncation algorithm. A full stack trace parser would be more accurate but vastly more complex for marginal gain.

**Range-based cut over line-by-line keep-set.** The initial implementation used a keep-set that retained individual lines and inserted omission messages per-frame. Go's two-line frame pairs (function + source location) caused duplicate omission messages because the non-frame line between pairs reset the insertion flag. The range-based approach replaces everything between the last kept first-frame and the first kept last-frame wholesale — one cut, one omission message, no interleaving bugs.

**Field stripping re-serializes JSON.** `stripFields` parses into `map[string]any`, deletes keys, re-marshals. This reorders keys (Go maps are unordered) but produces valid, compact JSON. The alternative — regex-based key removal — would be fragile with nested objects and escaped characters. The parse+marshal cost is acceptable because it short-circuits when no keys match.

**`FormatEvent` returns a copy, not a mutation.** `CanonicalEvent` is a value type with no pointer fields. `FormatEvent` takes it by value and returns a modified copy. The original event remains unmodified — safe for concurrent use by dedup, logging, and output.

**Dedup key is `Type.Category`, not raw text.** Semantic deduplication. Two `ERROR.connection_failure` events with different messages are "the same" from a downstream agent's perspective. Raw-text dedup would miss semantic duplicates (same error, different formatting). The aggressive collapsing trades individual event detail for dramatic token reduction during error storms.

**Streaming buffer uses `time.Timer`, not `time.Ticker`.** A ticker fires continuously regardless of whether events are arriving. A timer starts only when the first event arrives and fires once after the window — no wasted CPU when the stream is idle. After flush, the timer is nil until the next event.

**`Pipeline.New` uses functional options, not a config struct.** The `WithDedup(d, window)` option pattern keeps the zero-value pipeline (no dedup) working without any configuration. Adding future options (metrics, filtering, backpressure) won't break existing call sites.

**Stack trace truncation and character truncation are mutually exclusive.** Both serve the same purpose: reducing token count. When stack trace truncation fires (detected frames, returned a different string), character truncation is skipped. This prevents the omission message from being clipped by the character limit — a bug that existed in v0.4.0 and was fixed in v0.4.1.

**`getenvDuration` treats `"0"` as explicit disable.** `time.ParseDuration("0")` would return `0` anyway, but checking for `"0"` before parsing makes the semantics explicit in code: `LUMBER_DEDUP_WINDOW=0` intentionally disables dedup, it's not a parse error.

**Token estimation uses a whitespace heuristic, not a real BPE tokenizer.** `EstimateTokens` splits on whitespace and applies a 1.3× subword expansion factor. This is accurate within ~20% — more than sufficient for measuring compaction *ratios* in tests. A real tokenizer (tiktoken, sentencepiece) would add a dependency for negligible accuracy gain in ratio measurement.

---

## Known Limitations

1. **Token estimation is approximate.** The 1.3× whitespace heuristic doesn't account for actual BPE tokenization patterns. Real token counts may differ by up to 20%. Sufficient for ratio measurement, not for precise token budgeting.

2. **Stack trace patterns are Java/Go only.** Python tracebacks (`File "...", line N`), Node.js (`at Object.<anonymous>`), and Rust backtraces (`<...>::...`) are not detected. The regex list is extensible but currently limited.

3. **Field stripping is top-level only.** Nested JSON objects are not traversed. A `trace_id` inside a nested `metadata` object won't be stripped. This handles the common case (flat structured logs) but misses deeply nested formats.

4. **Dedup is in-memory only.** No state survives process restart. The dedup window is bounded by the process lifetime and the configured duration. Long-running error storms that span restarts will produce duplicate counted summaries.

5. **Streaming dedup buffer is unbounded.** During the dedup window, events accumulate without limit. A burst of 10,000 events in a 5-second window will all be buffered in memory. Bounded buffering with backpressure is a Phase 5 concern.

6. **No per-field verbosity control.** A single global verbosity setting controls all compaction behavior. There's no way to say "strip trace_id but keep request_id" or "truncate stack traces but not request bodies." The `WithStripFields` option provides field-level control for stripping, but truncation and summarization are global.

7. **JSON key reordering on strip.** `stripFields` re-marshals via `json.Marshal`, which produces keys in Go's map iteration order (effectively random). If downstream consumers depend on key ordering, this could be surprising. In practice, JSON key order is not guaranteed by the spec and well-behaved consumers don't depend on it.

8. **No output destinations beyond stdout.** The `Output` interface is extensible, but only `stdout.Output` is implemented. File, webhook, and gRPC output are future work.
