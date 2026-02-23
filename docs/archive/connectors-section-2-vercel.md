# Section 2: Vercel Connector — Completion Notes

## What Was Done

### 2.1 — Response types

Defined unexported types in `vercel.go` matching the Vercel REST API response shape:
- `logsResponse` with `Data []logEntry` and `Pagination pagination`
- `logEntry` with `ID`, `Message`, `Timestamp` (unix ms), `Source`, `Level`, and optional `Proxy *proxyInfo`
- `proxyInfo` with `StatusCode`, `Path`, `Method`, `Host`
- `pagination` with `Next` cursor string

### 2.2 — `toRawLog(entry logEntry) model.RawLog`

Maps Vercel log entries to the shared `RawLog` type:
- `Timestamp`: `time.UnixMilli(entry.Timestamp)`
- `Source`: `"vercel"` (hardcoded)
- `Raw`: `entry.Message`
- `Metadata`: always includes `level`, `source`, `id`. When `entry.Proxy` is non-nil, also includes `status_code`, `path`, `method`, `host`.

### 2.3 — `Query()`

- Extracts `project_id` from `cfg.Extra` (required, returns descriptive error if missing)
- Optionally reads `team_id` from `cfg.Extra` → sets `teamId` query param
- Creates `httpclient.Client` with `cfg.Endpoint` (falls back to `https://api.vercel.com`)
- Builds query params: `from`/`to` as unix milliseconds from `params.Start`/`params.End` (if non-zero)
- Pagination loop: follows `pagination.Next` cursor until empty or `params.Limit` reached
- All errors wrapped with `"vercel connector:"` prefix

### 2.4 — `Stream()`

- Same config extraction as `Query` (project_id required, team_id optional)
- Parses `poll_interval` from `cfg.Extra` via `time.ParseDuration` (default 5s)
- Creates buffered channel (capacity 64)
- Goroutine: initial poll immediately, then ticker-based loop
- `poll()` helper: fetches one page, sends entries to channel with context-aware select, logs errors to stderr without crashing, returns updated cursor
- Context cancellation closes channel and exits goroutine

### 2.5 — Tests

8 tests in `vercel_test.go`, all using `httptest.NewServer`:

| Test | What it verifies |
|------|-----------------|
| `TestToRawLog` | Timestamp conversion, all metadata fields including proxy |
| `TestToRawLog_NoProxy` | Proxy fields absent when proxy is nil |
| `TestQuery_Success` | Correct path, auth header, RawLog mapping |
| `TestQuery_Pagination` | Two pages via cursor, both collected |
| `TestQuery_MissingProjectID` | Descriptive error returned |
| `TestQuery_APIError` | 401 propagates as error |
| `TestStream_ReceivesLogs` | Logs arrive on channel across multiple polls (50ms interval) |
| `TestStream_ContextCancel` | Channel closes promptly after cancel |

## Design Decisions

- **`poll()` extracted as a helper** — keeps the goroutine loop clean and testable. Takes the client, path, and channel as params, returns updated cursor.
- **Initial poll before ticker** — `Stream()` fires immediately rather than waiting for the first tick interval. Provides faster first-log delivery.
- **Error logging to stderr, not channel** — API errors during `Stream()` are logged but don't close the channel. Transient failures (network blips, rate limits) are retried by the httpclient, and the poll loop continues on the next tick.
- **`poll_interval` parsed via `time.ParseDuration`** — supports `5s`, `500ms`, `1m`, etc. More flexible than seconds-only.

## Files

| File | Action |
|------|--------|
| `internal/connector/vercel/vercel.go` | Replaced stub |
| `internal/connector/vercel/vercel_test.go` | Created |

## Verification

```
$ go test ./internal/connector/vercel/... -v -count=1
=== RUN   TestToRawLog              --- PASS
=== RUN   TestToRawLog_NoProxy      --- PASS
=== RUN   TestQuery_Success         --- PASS
=== RUN   TestQuery_Pagination      --- PASS
=== RUN   TestQuery_MissingProjectID --- PASS
=== RUN   TestQuery_APIError        --- PASS
=== RUN   TestStream_ReceivesLogs   --- PASS (0.05s)
=== RUN   TestStream_ContextCancel  --- PASS
PASS

$ go build ./cmd/lumber  # compiles cleanly
```
