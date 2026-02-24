# Phase 6, Section 2: Edge Case Hardening — Completion Notes

**Completed:** 2026-02-24
**Phase:** 6 (Beta Validation & Polish)
**Depends on:** None (engine-level change, no dependencies on Section 1)

## Summary

Fixed two known classification edge cases from Phase 3's "Known limitations" list:
1. UNCLASSIFIED events now get `"warning"` severity instead of empty string
2. Empty/whitespace-only logs return UNCLASSIFIED with `category: "empty_input"` and zero confidence, bypassing the embedding call entirely

Both fixes apply to `Process()` and `ProcessBatch()`. Two new tests run without ONNX using a `panicEmbedder` mock.

## What Changed

### Modified Files

| File | Lines changed | What |
|------|---------------|------|
| `internal/engine/engine.go` | +25 | Early return for empty input, UNCLASSIFIED severity default, `emptyInputEvent` helper |
| `internal/engine/engine_test.go` | +50 | 2 new no-ONNX tests, updated 2 existing tests with assertions |

### `internal/engine/engine.go`

**Early return for empty/whitespace input:**

```go
if strings.TrimSpace(raw.Raw) == "" {
    return emptyInputEvent(raw), nil
}
```

Added at the top of `Process()` before the `Embed()` call. In `ProcessBatch()`, added in the per-event loop before `Classify()`.

**UNCLASSIFIED severity default:**

```go
severity := result.Label.Severity
if eventType == "UNCLASSIFIED" && severity == "" {
    severity = "warning"
}
```

Added in both `Process()` and `ProcessBatch()` after classification. Handles the case where the classifier returns UNCLASSIFIED (confidence below threshold) — the fresh `EmbeddedLabel{Path: "UNCLASSIFIED"}` has zero-value fields, so `Severity` was `""`.

**`emptyInputEvent` helper:**

```go
func emptyInputEvent(raw model.RawLog) model.CanonicalEvent {
    return model.CanonicalEvent{
        Type:       "UNCLASSIFIED",
        Category:   "empty_input",
        Severity:   "warning",
        Timestamp:  raw.Timestamp,
        Confidence: 0,
        Raw:        raw.Raw,
    }
}
```

DRYs the empty-input event creation between `Process()` and `ProcessBatch()`.

### Tests Added/Updated (4)

| Test | ONNX needed? | What it validates |
|------|-------------|-------------------|
| `TestProcessEmptyLog_ReturnsUnclassified` | No | Empty string → UNCLASSIFIED, empty_input category, warning severity, zero confidence, timestamp preserved. Uses `panicEmbedder` to prove embedder is never called |
| `TestProcessWhitespaceLog_ReturnsUnclassified` | No | `"   \n\t  "` → same UNCLASSIFIED behavior. Uses `panicEmbedder` |
| `TestProcessEmptyLog` (updated) | Yes | Now asserts UNCLASSIFIED type and warning severity instead of just "should not crash" |
| `TestProcessWhitespaceLog` (updated) | Yes | Same updated assertions |

### `panicEmbedder`

```go
type panicEmbedder struct{}
func (p panicEmbedder) Embed(string) ([]float32, error)          { panic("Embed called on empty input") }
func (p panicEmbedder) EmbedBatch([]string) ([][]float32, error) { panic("EmbedBatch called on empty input") }
func (p panicEmbedder) Close() error                              { return nil }
```

Satisfies `embedder.Embedder`. Panics if any embedding method is called, proving the early return bypasses the embedding path. Used with `engine.New(panicEmbedder{}, nil, nil, nil)` — the engine doesn't validate its components at construction time, so nil taxonomy/classifier/compactor is fine when the early return prevents them from being used.

## Design Decisions

- **`"warning"` for UNCLASSIFIED severity** — UNCLASSIFIED means the system couldn't determine the log type. That's worth attention (something unexpected is happening) but not an error in the log source itself. `"info"` would suppress it; `"error"` would be misleading.
- **`"empty_input"` as category** — distinguishes empty-input UNCLASSIFIED from low-confidence UNCLASSIFIED. Downstream consumers can filter or ignore empty-input events specifically.
- **`Confidence: 0` for empty input** — no classification was attempted, so confidence is zero (not some arbitrary score from embedding `[CLS][SEP]`).
- **Early return in `Process()`, post-classify override in `ProcessBatch()`** — `Process()` avoids the embedding call entirely (single-event hot path optimization). `ProcessBatch()` still passes empty strings through `EmbedBatch()` (they're part of the batch) but overrides the result in the per-event loop. Filtering empty strings from the batch would complicate index mapping for minimal gain.
- **`panicEmbedder` tests run on any machine** — the most important edge case tests (empty/whitespace) no longer require ONNX model files. Previously they were ONNX-dependent and would skip on most CI/dev machines.

## Verification

```
go test -v -count=1 ./internal/engine/...     # 2 new tests PASS, ONNX tests skip
go test ./...                                  # full suite passes (including pipeline)
go build ./cmd/lumber                          # compiles
go vet ./internal/engine/...                   # clean
```
