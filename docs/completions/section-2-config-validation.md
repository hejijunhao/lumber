# Section 2: Config Validation — Completion Notes

**Completed:** 2026-02-23
**Phase:** 5 (Pipeline Integration & Resilience)
**Depends on:** Section 1 (Structured Logging) — uses `slog.Error` in main.go for validation failure reporting

## Summary

Added `Validate() error` method to `Config` that checks all configuration fields at startup. Called in `main.go` immediately after `Load()` and `logging.Init()`, before any component initialization. Collects all errors into a single message — users see every misconfiguration at once instead of fixing them one at a time.

## What Changed

### Modified Files

| File | Change | Why |
|------|--------|-----|
| `internal/config/config.go` | Added `Validate() error` method (~35 lines), added `"fmt"` import | Fail-fast validation before expensive ONNX model loading |
| `internal/config/config_test.go` | Added `validConfig()` helper + 7 validation tests, added `"path/filepath"` and `"strings"` imports | Full coverage of each validation rule plus multi-error aggregation |
| `cmd/lumber/main.go` | Added `cfg.Validate()` call after `logging.Init()`, before embedder creation | Early exit with structured error if config is invalid |

### Validation Rules

| Rule | Check | Error message contains |
|------|-------|----------------------|
| API key required | Provider set + APIKey empty | `LUMBER_API_KEY` |
| Model file exists | `os.Stat()` on ModelPath | `model file not found` |
| Vocab file exists | `os.Stat()` on VocabPath | `vocab file not found` |
| Projection file exists | `os.Stat()` on ProjectionPath | `projection file not found` |
| Confidence threshold | Must be in [0, 1] | `confidence threshold` |
| Verbosity enum | Must be minimal\|standard\|full | `verbosity` |
| Dedup window | Must be non-negative | `dedup window` |

### Tests Added (7)

| Test | What it validates |
|------|-------------------|
| `TestValidate_ValidConfig` | Valid config with real temp files returns nil |
| `TestValidate_BadConfidenceThreshold` | 1.5 produces error mentioning "confidence" |
| `TestValidate_BadVerbosity` | `"verbose"` produces error mentioning "verbosity" |
| `TestValidate_NegativeDedupWindow` | -1s produces error mentioning "dedup" |
| `TestValidate_MissingModelFile` | Non-existent path produces error mentioning "model" |
| `TestValidate_MissingAPIKey` | Provider set with empty APIKey produces error mentioning "LUMBER_API_KEY" |
| `TestValidate_MultipleErrors` | Three bad fields all appear in a single error message |

## Design Decisions

- **Collect all errors, not just the first** — `var errs []string` accumulates every validation failure, joined with `\n  - ` for readability. This prevents the frustrating fix-one-rerun-fix-another loop.
- **No import of `connector` package** — avoids import cycle. Provider name validation stays in `main.go` where `connector.Get()` already handles it with a clear error.
- **`os.Stat` for file checks** — lightweight existence check. Doesn't open or read files. `os.IsNotExist(err)` correctly handles permission errors (those are not "not found").
- **Zero-value DedupWindow is valid** — 0 means "disabled", which is intentional. Only negative values are rejected.

## Verification

```
go test ./internal/config/...    # 16 tests pass (9 existing + 7 new)
go build ./cmd/lumber            # compiles cleanly
go test ./...                    # full suite passes
```
