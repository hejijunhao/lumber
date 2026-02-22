# Classification Pipeline — Section 4: Confidence Threshold Tuning

**Completed:** 2026-02-21

## What changed

### `internal/config/config.go`
- `ConfidenceThreshold` now reads from `LUMBER_CONFIDENCE_THRESHOLD` env var (default: 0.5)
- Added `getenvFloat()` helper for float64 env var parsing with fallback

### `internal/engine/engine_test.go`
- Enhanced `TestCorpusConfidenceDistribution` with a threshold sweep (0.50–0.85) showing correct/incorrect/net accuracy at each level
- Added gap analysis with automatic suggestion when distributions are separable

## Threshold analysis

### Confidence distributions (threshold=0.5, all entries classified)

| | n | mean | min | max |
|---|---|------|-----|-----|
| Correct | 93 | 0.781 | 0.662 | 0.883 |
| Incorrect | 11 | 0.749 | 0.657 | 0.847 |

### Threshold sweep

| Threshold | Correct kept | Incorrect kept | Net Accuracy |
|-----------|-------------|----------------|--------------|
| 0.50 | 93 | 11 | 89.4% |
| 0.55 | 93 | 11 | 89.4% |
| 0.60 | 93 | 11 | 89.4% |
| 0.65 | 93 | 11 | 89.4% |
| 0.70 | 85 | 9 | 90.4% |
| 0.75 | 69 | 5 | 93.2% |
| 0.80 | 38 | 2 | 95.0% |
| 0.85 | 6 | 0 | 100.0% |

### Decision: keep default at 0.5

The correct and incorrect confidence distributions overlap significantly (gap = -0.087). Raising the threshold would reject misclassifications but also reject many correct classifications:
- At 0.70: rejects 2 incorrect but also 8 correct (net: 90.4%, but 8 entries become UNCLASSIFIED)
- At 0.75: rejects 6 incorrect but also 24 correct (unacceptable)

The 0.5 default is appropriate:
- All 93 correct classifications are kept (none score below 0.662)
- All 11 misclassifications score above 0.657 — they're confident but wrong
- The gibberish test scored 0.546, which would correctly pass at 0.5 but be rejected if we raise to ~0.55

**Primary improvement lever is taxonomy description tuning (Section 5), not threshold adjustment.** The misclassifications score high confidence because the wrong category's description is semantically closer than the right one — fixing the descriptions will fix the misclassifications without needing threshold changes.

## Files changed

- `internal/config/config.go` — `LUMBER_CONFIDENCE_THRESHOLD` env var, `getenvFloat()` helper
- `internal/engine/engine_test.go` — threshold sweep and gap analysis in confidence distribution test
