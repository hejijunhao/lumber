# Classification Pipeline — Section 6: Edge Cases & Robustness

**Completed:** 2026-02-21

## What was built

7 edge case tests added to `internal/engine/engine_test.go` verifying the pipeline handles degenerate inputs gracefully.

## Tests added

| Test | Input | Result |
|------|-------|--------|
| `TestProcessEmptyLog` | `Raw: ""` | No crash. Classified as REQUEST.success (conf=0.609) — arbitrary but safe |
| `TestProcessWhitespaceLog` | `Raw: "   \n\t  "` | No crash. Same as empty — tokenizer strips to [CLS][SEP] |
| `TestProcessVeryLongLog` | 3663-char log with signal at start | ERROR.connection_failure (conf=0.735). Truncation to 128 tokens preserves signal |
| `TestProcessBinaryContent` | Null bytes, invalid UTF-8 | No crash. Tokenizer handles gracefully via control char stripping |
| `TestProcessTimestampPreservation` | Specific timestamp with nanoseconds | Exact match preserved through pipeline |
| `TestProcessZeroTimestamp` | Zero-value `time.Time` | Zero value preserved (not modified to `time.Now()` or similar) |
| `TestProcessMetadataNotInOutput` | RawLog with Source + Metadata populated | No crash. Metadata not surfaced in CanonicalEvent (by design for Phase 2) |

## Findings

- **Empty/whitespace inputs:** The tokenizer produces `[CLS][SEP]` with all-padding attention mask. Mean pooling over a 2-token sequence still produces a valid vector. Classification is arbitrary but the pipeline doesn't crash or return errors.
- **Long inputs:** The 128-token truncation in the tokenizer works as designed. Since log lines typically have their signal (error type, status code, etc.) in the first ~20 tokens, classification remains accurate even for very long entries.
- **Binary/non-UTF-8:** The tokenizer's `cleanText()` function strips control characters and the WordPiece lookup falls back to `[UNK]` for unknown byte sequences. No panics.
- **Timestamp:** Faithfully copied from `RawLog.Timestamp` to `CanonicalEvent.Timestamp` with no modification, including zero values.
- **Metadata:** `RawLog.Metadata` and `RawLog.Source` are present on input but not surfaced in `CanonicalEvent`. The pipeline ignores them without error. Metadata passthrough is a future concern.

## Files changed

- `internal/engine/engine_test.go` — 7 new edge case tests (added `strings` import)
