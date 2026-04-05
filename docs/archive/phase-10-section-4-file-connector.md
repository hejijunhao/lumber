# Phase 10, Section 4: File Connector

**Completed:** 2026-04-05

**Scope:** Added a `file` connector that reads log lines from a file on disk. Enables `lumber -connector file -file app.log` workflows — classify local log files without any cloud credentials.

**Plan:** `docs/executing/phase-10-cli-wizard.md`, Section 4

---

## What Was Done

### 1. Created `internal/connector/file/file.go`

Implements `connector.Connector` for file-based log ingestion.

**Stream behavior:**
- Opens the file at `cfg.Extra["file"]`
- Reads lines via `bufio.Scanner` in a goroutine (same pattern as stdin connector)
- Each non-empty line becomes a `model.RawLog` with `Source: "file"`, `Metadata["file"]` set to the base filename
- Scanner buffer set to 1MB for long lines
- Closes channel on EOF or context cancellation
- Logs scanner errors via `slog.Warn`

**Query behavior:**
- Reads the file sequentially, returning up to `params.Limit` results
- `Start`/`End` time filters are ignored with a debug log (file lines have no inherent timestamp)
- Returns all lines if no limit is set

**Registration:** `init()` registers as `"file"` in the connector registry.

**Config wiring:** File path comes from `cfg.Extra["file"]`, validated by `resolveFilePath()` which returns a clear error if missing.

### 2. Created `internal/connector/file/file_test.go`

9 tests covering all behaviors:

| Test | What |
|------|------|
| `TestStream_ReadsFile` | 5-line file → 5 RawLogs with correct content and source |
| `TestStream_RespectsContextCancellation` | 10K-line file, cancel after 5 → clean shutdown |
| `TestQuery_WithLimit` | 10-line file, limit=3 → exactly 3 results |
| `TestStream_MissingFile` | Nonexistent path → error |
| `TestStream_EmptyFile` | Empty file → channel closes, 0 events |
| `TestStream_FileMetadata` | Verify `Metadata["file"]` contains the base filename |
| `TestStream_MissingFilePath` | No "file" key in Extra → clear config error |
| `TestQuery_ReadsAllWithoutLimit` | 3-line file, no limit → all 3 returned |
| `TestQuery_TimeFiltersIgnored` | Time range params → still returns all lines |

---

## Verification

- `go build ./...` — compiles cleanly
- `go test ./internal/connector/file/` — 9/9 pass

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/connector/file/file.go` | **new** | File connector: Stream, Query, registration |
| `internal/connector/file/file_test.go` | **new** | 9 tests covering all behaviors |

**New files: 2. Total: 2.**

---

## Design Decisions

- **Query supports limit but not time filters:** File lines have no inherent timestamps — Lumber assigns `time.Now()` at read time. Time-range filtering against that is meaningless (every line would have the same timestamp). The limit parameter is useful though, so it's supported. Time filters are silently ignored with a debug log rather than erroring, since the caller (pipeline) may pass them generically.
- **`Metadata["file"]` uses `filepath.Base`:** Only the filename, not the full path, is stored in metadata. Full paths leak machine-specific directory structure into the canonical output. The base filename is what downstream consumers need to correlate events to a source file.
- **File is opened in Stream, closed in goroutine:** The `defer f.Close()` is inside the goroutine, not in Stream(). This ensures the file handle stays open for the entire scan and is released when the channel closes — matching the lifecycle of the goroutine.
- **Shared patterns with stdin connector:** Both use 1MB scanner buffers, channel buffer of 64, empty line skipping, and the same goroutine-with-defer-close pattern. This consistency is intentional — they're both line-oriented local connectors.
