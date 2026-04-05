# Phase 10, Section 3: Stdin Connector

**Completed:** 2026-04-05

**Scope:** Added a `stdin` connector that reads log lines from standard input, enabling `cat app.log | lumber` and piped-input workflows. This is Lumber's first local (non-cloud) connector — no API key, no network calls.

**Plan:** `docs/executing/phase-10-cli-wizard.md`, Section 3

---

## What Was Done

### 1. Created `internal/connector/stdin/stdin.go`

Implements the `connector.Connector` interface for stdin-based log ingestion.

**Stream behavior:**
- Opens a `bufio.Scanner` on the configured `io.Reader` (defaults to `os.Stdin`)
- Reads lines in a goroutine, sends each non-empty line as a `model.RawLog` on the output channel
- Each `RawLog` has `Source: "stdin"`, `Timestamp: time.Now()`, and `Raw` set to the line text
- Skips empty lines to avoid sending blank events
- Scanner buffer set to 1MB via `Scanner.Buffer()` to handle long log lines (stack traces can exceed the 64KB default)
- Closes channel on EOF or context cancellation

**Query behavior:**
- Returns `fmt.Errorf("stdin connector does not support query mode")` — stdin is inherently streaming with no historical query concept

**Registration:**
- `init()` registers as `"stdin"` in the connector registry, following the same pattern as Vercel, Fly.io, and Supabase connectors

**Testability:**
- Accepts an `io.Reader` via `WithReader(r)` option — default is `os.Stdin`, tests pass a `strings.Reader`
- `New(opts ...Option)` constructor for test use; `init()` registration uses the default

### 2. Created `internal/connector/stdin/stdin_test.go`

6 tests covering all behaviors:

| Test | What |
|------|------|
| `TestStream_ReadsLines` | 3-line input → 3 RawLogs with correct Raw, Source, Timestamp |
| `TestStream_RespectsContextCancellation` | Cancel context → channel closes promptly |
| `TestStream_EmptyInput` | Empty reader → channel closes immediately, 0 events |
| `TestStream_SkipsEmptyLines` | Input with blank lines → only non-empty lines emitted |
| `TestQuery_ReturnsError` | Query() → error with "does not support query mode" |
| `TestStream_LongLines` | 100KB line (exceeds default 64KB limit) → read completely |

---

## Verification

- `go build ./...` — compiles cleanly
- `go test ./internal/connector/stdin/` — 6/6 pass
- Connector registered as `"stdin"` in the global registry via `init()`

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/connector/stdin/stdin.go` | **new** | Stdin connector: Stream, Query, registration |
| `internal/connector/stdin/stdin_test.go` | **new** | 6 tests covering all behaviors |

**New files: 2. Total: 2.**

---

## Design Decisions

- **`io.Reader` injection via option:** Hardcoding `os.Stdin` would make the connector untestable without OS-level pipe manipulation. The `WithReader()` option follows the standard Go pattern for testable I/O — the `init()` registration uses the real stdin, tests inject `strings.Reader`.
- **1MB scanner buffer:** The default `bufio.Scanner` limit is 64KB. Stack traces, JSON-formatted logs, and base64 payloads can easily exceed this. 1MB handles these cases while keeping memory reasonable.
- **Empty line skipping:** Blank lines in log files are noise — they'd produce empty `RawLog` entries that the classification engine would process pointlessly. Filtering at the connector level is the right place.
- **Channel buffer of 64:** Matches the existing pattern in the Fly.io connector. Provides enough headroom for the scanner to read ahead while the engine processes.
