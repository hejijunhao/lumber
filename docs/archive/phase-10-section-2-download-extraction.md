# Phase 10, Section 2: Extract Download Logic to `internal/download/`

**Completed:** 2026-04-05

**Scope:** Extracted the core download infrastructure from `pkg/lumber/` (library-only) to `internal/download/` (shared by both library and CLI). This unblocks the CLI wizard (Section 6) from triggering model downloads without importing the public API package.

**Plan:** `docs/executing/phase-10-cli-wizard.md`, Section 2

---

## What Was Done

### 1. Created `internal/download/download.go`

**New file** containing all download machinery, now exported:

| Function | Purpose |
|----------|---------|
| `DownloadModels(destDir)` | Downloads 5 model files, skipping valid cached copies |
| `DownloadORT(destDir)` | Downloads platform-specific ONNX Runtime library from GitHub Releases |
| `OrtPlatform()` | Returns archive suffix + library filename for current `GOOS`/`GOARCH` |
| `FileValid(path, sha256)` | Existence + checksum verification |
| `DownloadFile(url, dest, sha256)` | HTTP download with atomic write + hash-while-write |
| `AtomicWriteFromReader(dest, r)` | Temp file + `os.Rename` for concurrency safety |
| `DefaultCacheDir()` | Resolves `$LUMBER_CACHE_DIR` or OS-native cache path |

Also exported: `ModelFile` struct, `ModelFiles` slice, `HFBase` and `ORTVersion` constants.

Functions that were unexported in `pkg/lumber/` (e.g. `downloadModels`, `fileValid`) are now exported in `internal/download/` (e.g. `DownloadModels`, `FileValid`). Since `internal/` packages are invisible to external consumers, this is safe — only code within `github.com/kaminocorp/lumber` can import it.

### 2. Rewrote `pkg/lumber/download.go` as Thin Wrappers

All 6 functions now delegate to `internal/download`:

```go
func downloadModels(destDir string) error { return download.DownloadModels(destDir) }
func fileValid(path, sha string) bool      { return download.FileValid(path, sha) }
// ... etc
```

The functions remain unexported in `pkg/lumber/` — they're called by `lumber.go:New()` internally. The public API (`WithAutoDownload()`, `WithCacheDir()`, etc.) is completely unchanged.

### 3. Rewrote `pkg/lumber/cache.go` as One-Liner

```go
func defaultCacheDir() (string, error) { return download.DefaultCacheDir() }
```

### 4. Created `internal/download/download_test.go`

10 tests migrated from `pkg/lumber/download_test.go`, now testing the exported functions directly:

| Test | What |
|------|------|
| `TestDefaultCacheDir` | `$LUMBER_CACHE_DIR` override + OS fallback |
| `TestFileValid` | Non-existent, no-checksum, matching, mismatched |
| `TestDownloadFile` | Happy path with SHA256 via httptest |
| `TestDownloadFile_ChecksumMismatch` | Corrupt download rejected, temp cleaned |
| `TestDownloadFile_HTTPError` | HTTP 404 surfaces as error |
| `TestDownloadFile_SkipsIfCached` | Valid cached file not re-downloaded |
| `TestDownloadFile_SubdirectoryCreated` | Nested parent dirs created |
| `TestDownloadFile_CorruptCacheRedownloaded` | Corrupt cache detected + replaced |
| `TestOrtPlatform` | Platform detection for current OS/arch |
| `TestAtomicWriteFromReader` | Temp file + rename correctness |

### 5. Rewrote `pkg/lumber/download_test.go` as Wrapper Tests

5 focused tests confirming delegation works:

| Test | What |
|------|------|
| `TestDefaultCacheDir_Wrapper` | Cache dir wrapper delegates correctly |
| `TestFileValid_Wrapper` | Checksum wrapper delegates correctly |
| `TestDownloadFile_Wrapper` | Download wrapper delegates correctly |
| `TestOrtPlatform_Wrapper` | Platform wrapper delegates correctly |
| `TestAtomicWriteFromReader_Wrapper` | Atomic write wrapper delegates correctly |

---

## Verification

- `go build ./...` — compiles cleanly
- `go test ./internal/download/` — 10/10 pass
- `go test ./pkg/lumber/` — 5/5 wrapper tests pass
- `go test ./...` — full suite green (all 24 packages)
- `pkg/lumber` public API unchanged — `WithAutoDownload()` still calls `downloadModels()` → `download.DownloadModels()`

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/download/download.go` | **new** | Core download logic (exported), cache dir resolution |
| `internal/download/download_test.go` | **new** | 10 tests covering all download behaviors |
| `pkg/lumber/download.go` | rewritten | Thin wrappers delegating to `internal/download` |
| `pkg/lumber/cache.go` | rewritten | One-liner delegating to `internal/download` |
| `pkg/lumber/download_test.go` | rewritten | 5 wrapper-validation tests |

**New files: 2. Rewritten files: 3. Total: 5.**

---

## Design Decisions

- **Why `internal/download/` and not a top-level `download/` package:** Go's `internal/` convention restricts import visibility to the parent module. This gives both `pkg/lumber/` and `cmd/lumber/` access while preventing external consumers from depending on download internals (URLs, checksums, file layout). External consumers use `WithAutoDownload()`.
- **Why export everything in `internal/download/`:** Since the package is already access-controlled by `internal/`, there's no reason to keep functions unexported. Exporting them gives both `pkg/lumber/` wrappers and the future CLI wizard (`cmd/lumber/`) clean access without relying on package-internal hacks.
- **Why keep wrappers in `pkg/lumber/` instead of importing directly in `lumber.go`:** The wrappers maintain the same unexported function signatures that `lumber.go:New()` already calls (`downloadModels`, `downloadORT`, `defaultCacheDir`). This means `lumber.go` required zero changes — the refactor is invisible to the file that orchestrates initialization.
- **Why rewrite `pkg/lumber/download_test.go` rather than delete it:** The wrapper tests serve as a contract test — if someone accidentally removes a wrapper or changes its signature, these tests catch it. They're lightweight (5 tests, ~60 lines) and run in <1s.
