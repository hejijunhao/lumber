# Section 4: Supabase Connector — Completion Notes

## What Was Done

### 4.1 — Registration

Created `internal/connector/supabase/supabase.go`, registered as `"supabase"` via `init()`.

### 4.2 — SQL builder and table allow-list

- `allowedTables` map covers all 7 Supabase log tables (4 default + 3 opt-in)
- `buildSQL(table, fromMicros, toMicros) (string, error)` — generates `SELECT id, timestamp, event_message FROM {table} WHERE timestamp >= {from} AND timestamp < {to} ORDER BY timestamp ASC LIMIT 1000`
- Table name validated against allow-list before interpolation — rejects anything not in the list, preventing SQL injection
- `defaultTables`: `edge_logs`, `postgres_logs`, `auth_logs`, `function_logs`

### 4.3 — Response type

`logsResponse` with `Result []map[string]any` — uses untyped maps because schema varies per table. Known fields (`id`, `timestamp`, `event_message`) are extracted by name.

### 4.4 — `toRawLog(row map[string]any, table string) model.RawLog`

- `Timestamp`: extracts `row["timestamp"]` as `float64` (JSON number), converts microseconds → `time.Unix(sec, usecRemainder*1000)`
- `Source`: `"supabase"`
- `Raw`: `row["event_message"]` as string
- `Metadata`: `{"table": table}` plus all other row fields except `event_message` (avoids duplication with `Raw`)

### 4.5 — `Query()`

- Extracts `project_ref` from `cfg.Extra` (required)
- Parses `tables` from `cfg.Extra["tables"]` (comma-separated, trimmed). Falls back to 4 default tables.
- Path: `/v1/projects/{ref}/analytics/endpoints/logs.all`
- **Default time range**: last 1 hour if both Start and End are zero
- **24-hour window chunking**: ranges exceeding 24h are split into sequential chunks. Each chunk queries all configured tables.
- Query params: `?sql=...&iso_timestamp_start=...&iso_timestamp_end=...` (ISO timestamps in RFC 3339)
- Results from all tables and chunks merged, sorted by timestamp, then truncated to `params.Limit`

### 4.6 — `Stream()`

- Default poll interval 10s (with 4 tables per poll = 24 req/min, within 120 req/min limit). Configurable via `cfg.Extra["poll_interval"]`.
- Maintains `lastMicros` cursor (microseconds). Initialized to `now - 1 minute`.
- Each tick: queries all configured tables from `lastMicros + 1` to `now`. Updates cursor to max timestamp seen.
- `pollStream()` helper iterates tables, queries each, sends results to channel. Per-table errors are logged and skipped (one bad table doesn't block others).

### 4.7 — Tests

11 tests in `supabase_test.go`:

| Test | What it verifies |
|------|-----------------|
| `TestBuildSQL` | Correct SQL output with table name and microsecond boundaries |
| `TestBuildSQL_InvalidTable` | Rejects table names not in allow-list |
| `TestToRawLog` | Microsecond timestamp conversion, event_message mapping, table in metadata, extra fields preserved, event_message not duplicated |
| `TestQuery_SingleTable` | Single table fixture, correct path and SQL, RawLog mapping |
| `TestQuery_MultipleTables` | Two tables, results interleaved and sorted by timestamp |
| `TestQuery_SlidingWindow` | 48-hour range produces 2 API calls (2 chunks × 1 table) |
| `TestQuery_MissingProjectRef` | Descriptive error |
| `TestQuery_DefaultTables` | No `tables` in Extra → 4 default tables queried |
| `TestQuery_CustomTables` | Comma-separated custom table list with whitespace trimming |
| `TestStream_ReceivesLogs` | Logs arrive on channel from poll |
| `TestStream_ContextCancel` | Channel closes promptly |

## Design Decisions

- **Allow-list for table names** — table names are interpolated directly into SQL. The allow-list is the only defense against injection. All 7 known Supabase log tables are included.
- **Per-table error isolation in `pollStream()`** — if one table returns an error (e.g., opt-in table not enabled), the other tables still get polled. Errors are logged, not fatal.
- **`event_message` excluded from metadata** — it's already in `Raw`, duplicating it would waste memory and confuse downstream consumers.
- **Default 10s poll interval** — conservative choice given 4 default tables × 1 req/table/poll = 24 req/min, well within the 120 req/min rate limit.
- **Cursor as max-timestamp-seen** — avoids missing logs that arrive out of order within the Supabase analytics pipeline. The `+1` offset prevents re-fetching the last seen entry.

## Files

| File | Action |
|------|--------|
| `internal/connector/supabase/supabase.go` | Created |
| `internal/connector/supabase/supabase_test.go` | Created |

## Verification

```
$ go test ./internal/connector/supabase/... -v -count=1
=== RUN   TestBuildSQL                --- PASS
=== RUN   TestBuildSQL_InvalidTable   --- PASS
=== RUN   TestToRawLog                --- PASS
=== RUN   TestQuery_SingleTable       --- PASS
=== RUN   TestQuery_MultipleTables    --- PASS
=== RUN   TestQuery_SlidingWindow     --- PASS
=== RUN   TestQuery_MissingProjectRef --- PASS
=== RUN   TestQuery_DefaultTables     --- PASS
=== RUN   TestQuery_CustomTables      --- PASS
=== RUN   TestStream_ReceivesLogs     --- PASS
=== RUN   TestStream_ContextCancel    --- PASS
PASS

$ go build ./cmd/lumber  # compiles cleanly
```
