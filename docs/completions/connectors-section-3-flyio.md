# Section 3: Fly.io Connector — Completion Notes

## What Was Done

### 3.1 — Registration

Created `internal/connector/flyio/flyio.go`, registered as `"flyio"` via `init()`.

### 3.2 — Response types

Unexported types matching Fly.io's HTTP logs API:
- `logsResponse` with `Data []logWrapper` and `Meta meta`
- `logWrapper` with `ID`, `Type`, and nested `Attributes logAttributes`
- `logAttributes` with `Timestamp` (RFC 3339 string), `Message`, `Level`, `Instance`, `Region`, `Meta map[string]any`
- `meta` with `NextToken` cursor string

### 3.3 — `toRawLog(w logWrapper) model.RawLog`

- `Timestamp`: parsed via `time.Parse(time.RFC3339Nano, ...)` — handles both second and sub-second precision
- `Source`: `"flyio"`
- `Raw`: `w.Attributes.Message`
- `Metadata`: `level`, `instance`, `region`, `id`, plus all entries from `w.Attributes.Meta` merged in

### 3.4 — `Query()`

- Extracts `app_name` from `cfg.Extra` (required)
- Path: `/api/v1/apps/{app_name}/logs`
- Paginates via `next_token` query param until empty or limit reached
- **Client-side time filter**: since Fly.io has no server-side time range filtering, entries outside `params.Start`/`params.End` are skipped after parsing. The filter uses `Before(Start)` and `!Before(End)` (i.e., `[Start, End)` — inclusive start, exclusive end)

### 3.5 — `Stream()`

Same poll-loop pattern as Vercel:
- Cursor via `next_token`
- Default 5s poll interval, configurable via `cfg.Extra["poll_interval"]` (parsed with `time.ParseDuration`)
- Buffered channel (64), immediate first poll, ticker-based loop
- Errors logged to stderr, polling continues

### 3.6 — Tests

7 tests in `flyio_test.go`:

| Test | What it verifies |
|------|-----------------|
| `TestToRawLog` | RFC 3339 timestamp parsing, all metadata fields, Meta map merge |
| `TestQuery_Success` | Correct path, auth header, RawLog mapping |
| `TestQuery_Pagination` | Two pages via `next_token` cursor |
| `TestQuery_ClientSideTimeFilter` | Entries outside `[Start, End)` excluded — 3 entries in, 1 returned |
| `TestQuery_MissingAppName` | Descriptive error |
| `TestStream_ReceivesLogs` | Logs arrive across multiple polls (50ms interval) |
| `TestStream_ContextCancel` | Channel closes promptly |

## Design Decisions

- **Client-side time filter with half-open interval `[Start, End)`** — matches standard time range semantics. A log exactly at `End` is excluded, preventing overlap when querying consecutive windows.
- **`Meta` map merged into top-level metadata** — Fly.io's `meta` field contains arbitrary provider-specific data. Flattening it into the RawLog metadata makes it accessible without nested lookups.
- **Structural parity with Vercel connector** — same poll helper pattern, same channel size, same error handling. Consistency across connectors reduces cognitive load.

## Files

| File | Action |
|------|--------|
| `internal/connector/flyio/flyio.go` | Created |
| `internal/connector/flyio/flyio_test.go` | Created |

## Verification

```
$ go test ./internal/connector/flyio/... -v -count=1
=== RUN   TestToRawLog                    --- PASS
=== RUN   TestQuery_Success               --- PASS
=== RUN   TestQuery_Pagination            --- PASS
=== RUN   TestQuery_ClientSideTimeFilter  --- PASS
=== RUN   TestQuery_MissingAppName        --- PASS
=== RUN   TestStream_ReceivesLogs         --- PASS (0.05s)
=== RUN   TestStream_ContextCancel        --- PASS
PASS

$ go build ./cmd/lumber  # compiles cleanly
```
