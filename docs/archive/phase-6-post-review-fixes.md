# Phase 6, Post-Review Fixes — Completion Notes

**Completed:** 2026-02-24
**Phase:** 6 (Beta Validation & Polish)
**Depends on:** Phase 6 Sections 1–5

## Summary

Post-review audit of the Phase 6 implementation identified 5 issues. All fixed with tests added where applicable. No behavioral regressions — full test suite passes.

## Fixes

### 1. Version output writes to stdout instead of stderr

**Problem:** `lumber -version` wrote to stderr (`fmt.Fprintf(os.Stderr, ...)`). POSIX convention and most CLI tools (git, docker, kubectl) write version info to stdout so it can be captured by scripts (`VERSION=$(lumber -version)`).

**Fix:** Changed to `fmt.Printf` in `cmd/lumber/main.go`.

| File | Change |
|------|--------|
| `cmd/lumber/main.go` | `fmt.Fprintf(os.Stderr, ...)` → `fmt.Printf(...)` |

### 2. Timer leak in `streamBuffer.flush()`

**Problem:** `flush()` set `b.timer = nil` without calling `timer.Stop()`. When flush was triggered by buffer-full before the timer fired, the old `time.Timer`'s internal goroutine leaked until it fired. Minor (fires once then GC'd), but a correctness issue.

**Fix:** Added `timer.Stop()` before nil assignment, guarded by nil check.

| File | Change |
|------|--------|
| `internal/pipeline/buffer.go` | Added `b.timer.Stop()` in `flush()` before setting `b.timer = nil` |

### 3. Silent `-from`/`-to` parse failures and missing query mode validation

**Problem:** Two related issues:
1. `LoadWithFlags()` silently swallowed RFC3339 parse errors for `-from` and `-to` flags. A user passing `-from "yesterday"` got no feedback — `QueryFrom` stayed at zero value.
2. `Validate()` didn't check that query mode has non-zero from/to. A user could run `lumber -mode query` and get confusing results.

**Fix:**
- `LoadWithFlags()` now collects parse errors into `Config.parseErrors` (unexported field).
- `Validate()` appends parse errors and checks that `QueryFrom` and `QueryTo` are non-zero when `Mode == "query"`.

| File | Change |
|------|--------|
| `internal/config/config.go` | Added `parseErrors []string` field, error collection in `-from`/`-to` handling, query mode from/to validation in `Validate()` |
| `internal/config/config_test.go` | 5 new tests: `TestValidate_QueryModeMissingFrom`, `TestValidate_QueryModeMissingTo`, `TestValidate_QueryModeMissingBoth`, `TestValidate_ParseErrors`, updated `TestValidate_QueryModeValid` to set from/to |

### 4. Corpus validation tests invisible to `go test ./...`

**Problem:** The `testdata` package lives in `internal/engine/testdata/`, which Go's toolchain skips by convention (`testdata/` directories are treated as fixture dirs, not packages). Running `go test ./...` never executed the corpus structural validation or coverage tests. They only ran when explicitly targeted by full module path.

**Fix:** Added wrapper tests in `internal/engine/engine_test.go` that call `testdata.LoadCorpus()` and validate:
- `TestCorpusStructure` — all 153 entries have non-empty raw, expected_type, expected_category, expected_severity
- `TestCorpusTaxonomyCoverage` — all 42 taxonomy leaves have at least 2 corpus entries

These run as part of `go test ./internal/engine/...` which is included in `./...`.

| File | Change |
|------|--------|
| `internal/engine/engine_test.go` | 2 new tests: `TestCorpusStructure`, `TestCorpusTaxonomyCoverage` |

### 5. `ProcessBatch` embedded empty strings unnecessarily

**Problem:** `ProcessBatch()` passed all raw texts (including empty/whitespace strings) through `EmbedBatch()`, then overrode empty results post-classification. This wasted ONNX inference cycles on inputs that would always be UNCLASSIFIED.

**Fix:** Refactored `ProcessBatch()` to:
1. Pre-scan inputs — empty/whitespace entries get `emptyInputEvent()` immediately
2. Collect non-empty texts and their original indices
3. Call `EmbedBatch()` only on non-empty texts
4. Map vectors back to original positions via index mapping

When all inputs are empty, `EmbedBatch` is never called.

| File | Change |
|------|--------|
| `internal/engine/engine.go` | Rewrote `ProcessBatch()` with pre-scan, index mapping, and filtered `EmbedBatch` call |
| `internal/engine/engine_test.go` | 1 new test: `TestProcessBatchAllEmpty_SkipsEmbedder` (uses `panicEmbedder`, no ONNX required) |

## Design Decisions

- **`parseErrors` as unexported field** — parse errors are collected during `LoadWithFlags()` and surfaced by `Validate()`. The field is unexported because it's an internal detail of the load→validate flow, not something callers should set directly.
- **Query mode requires both from and to** — rather than defaulting to "now" or "beginning of time", we require explicit time bounds. Implicit defaults for time ranges are a common source of unexpected behavior (e.g., querying all logs ever, or querying a zero-width window).
- **Corpus wrapper tests, not package rename** — renaming `testdata/` would break Go's convention for fixture directories. The wrapper approach keeps the convention while ensuring CI catches structural issues.
- **Index mapping in ProcessBatch** — slightly more code than the previous "embed everything, override later" approach, but avoids wasted inference. The mapping is O(n) and adds no measurable overhead.

## Tests Added

8 new tests across 2 packages:

| Package | Tests | What |
|---------|-------|------|
| `internal/config` | 5 | Query mode from/to validation (3), parse error surfacing (1), updated query valid (1) |
| `internal/engine` | 3 | Corpus structure (1), taxonomy coverage (1), batch all-empty skips embedder (1) |

## Verification

```
go test -count=1 ./...                    # all tests pass
go vet ./...                              # clean
go build ./cmd/lumber                     # compiles
```

## Files Changed

| File | Action |
|------|--------|
| `cmd/lumber/main.go` | modified |
| `internal/pipeline/buffer.go` | modified |
| `internal/config/config.go` | modified |
| `internal/config/config_test.go` | modified |
| `internal/engine/engine.go` | modified |
| `internal/engine/engine_test.go` | modified |

**New files: 0. Modified files: 6.**
