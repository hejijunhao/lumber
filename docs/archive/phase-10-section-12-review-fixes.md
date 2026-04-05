# Phase 10, Section 12 — Post-Review Fixes

**Date:** 2026-04-05

Post-implementation review of Phase 10. All issues identified during review, fixed, verified with `go vet` and full test suite.

## Issues Fixed

### Must-fix (blocked push)

#### 1. Context leak in `file_test.go` (`go vet` failure)

**File:** `internal/connector/file/file_test.go`
**Problem:** `context.WithCancel` in `TestStream_RespectsContextCancellation` did not guarantee `cancel()` on all code paths. If the channel closed before `count >= 5`, `cancel()` was never called, leaking the context.
**Fix:** Added `defer cancel()` immediately after `context.WithCancel`.

#### 2. Dead `os.Getenv("GOOS")` check in `wizard_test.go`

**File:** `internal/cli/wizard_test.go`
**Problem:** `ortLibNameForTest()` used `os.Getenv("GOOS")` to detect the platform. `GOOS` is a build-time Go constant (`runtime.GOOS`), not a runtime environment variable — `os.Getenv("GOOS")` returns `""` during normal execution. The fallback `/System` stat check happened to work on macOS, but the primary branch was dead code.
**Fix:** Replaced with `runtime.GOOS == "darwin"`. Removed the `/System` stat fallback.

#### 3. Silent time parse error discard in `wizard.go`

**File:** `internal/cli/wizard.go`
**Problem:** `promptQueryRange` validated time strings via `huh` form validators, then re-parsed them on lines 460-461 discarding errors with `_`. If `huh` returned a different value than what was validated (library bug, mutation), zero `time.Time` would silently pass through and surface much later as a confusing "missing -from" validation error.
**Fix:** Parse errors now propagate as `fmt.Errorf("parsing -from time: %w", parseErr)`.

### Should-fix (quality)

#### 4. Fragile path directory extraction in `config.go`

**File:** `internal/config/config.go`
**Problem:** File output directory validation used `strings.LastIndex(path, "/")` — a manual, Unix-only reimplementation of `filepath.Dir()`. Edge cases like `"./output.jsonl"` would produce `""` instead of `"."`, skipping validation entirely.
**Fix:** Replaced with `filepath.Dir()`. Added `filepath` import. Directory check now skips when result is `"."` (current directory, always exists) or `""`.

#### 5. Custom `containsStr`/`containsLoop` in `wizard_test.go`

**File:** `internal/cli/wizard_test.go`
**Problem:** `assertContains` helper used a hand-rolled `containsStr`/`containsLoop` (15 lines) instead of `strings.Contains` (stdlib, single call, better optimized).
**Fix:** Replaced with `strings.Contains`. Removed `containsStr` and `containsLoop`. Added `strings` import.

#### 6. Redundant `!cfg.ShowVersion` guard in `main.go`

**File:** `cmd/lumber/main.go`
**Problem:** The wizard/auto-detect block was gated by `cfg.Connector.Provider == "" && !cfg.ShowVersion`. The `!cfg.ShowVersion` check was dead — `ShowVersion` triggers `os.Exit(0)` on lines 41-44, so execution never reaches the wizard block when the flag is set.
**Fix:** Removed `&& !cfg.ShowVersion` from the condition.

### Minor fixes

#### 7. Stdin scanner error silently swallowed

**File:** `internal/connector/stdin/stdin.go`
**Problem:** The Stream goroutine did not check `scanner.Err()` after the scan loop. A read error (not EOF) would be silently lost. The file connector already logged scanner errors (line 70) — stdin was inconsistent.
**Fix:** Added `scanner.Err()` check after the loop, logging via `slog.Warn` (consistent with file connector). Added `log/slog` import.

#### 8. Hardcoded model paths in wizard diverged from `download.ModelFiles`

**File:** `internal/cli/wizard.go`
**Problem:** `promptModelDownload` hardcoded three path strings (`"model_quantized.onnx"`, `"vocab.txt"`, `"2_Dense/model.safetensors"`) that also exist in `download.ModelFiles[].RelPath`. If model files are added, renamed, or restructured in `download.ModelFiles`, the wizard would silently point at stale paths.
**Fix:** Config paths now derived by iterating `download.ModelFiles` and matching on `RelPath`, eliminating the duplicated strings.

## Verification

- `go vet ./...` — clean (zero output)
- `go build ./cmd/lumber/...` — clean
- `go test` — all 6 Phase 10 packages pass (31 tests)

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/connector/file/file_test.go` | modified | `defer cancel()` added |
| `internal/cli/wizard.go` | modified | Time parse errors propagated; model paths derived from `download.ModelFiles` |
| `internal/cli/wizard_test.go` | modified | `runtime.GOOS` replaces `os.Getenv("GOOS")`; `strings.Contains` replaces custom helpers |
| `internal/config/config.go` | modified | `filepath.Dir()` replaces manual string split |
| `cmd/lumber/main.go` | modified | Removed redundant `!cfg.ShowVersion` guard |
| `internal/connector/stdin/stdin.go` | modified | Scanner error logged after loop |

**New files: 1 (this doc). Modified files: 6. Total: 7.**
