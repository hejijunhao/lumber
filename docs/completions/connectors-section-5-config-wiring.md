# Section 5: Config Wiring and Main — Completion Notes

## What Was Done

### 5.1 — `Extra` field on `config.ConnectorConfig` + `loadConnectorExtra()`

Modified `internal/config/config.go`:

- Added `Extra map[string]string` to `ConnectorConfig`
- Added `loadConnectorExtra()` helper that reads provider-specific env vars into the map:

| Env Var | Extra Key | Used By |
|---------|-----------|---------|
| `LUMBER_VERCEL_PROJECT_ID` | `project_id` | Vercel |
| `LUMBER_VERCEL_TEAM_ID` | `team_id` | Vercel |
| `LUMBER_FLY_APP_NAME` | `app_name` | Fly.io |
| `LUMBER_SUPABASE_PROJECT_REF` | `project_ref` | Supabase |
| `LUMBER_SUPABASE_TABLES` | `tables` | Supabase |
| `LUMBER_POLL_INTERVAL` | `poll_interval` | All |

- Only non-empty values are added. Returns `nil` if no provider-specific vars are set (avoids allocating an empty map).

### 5.2 — `cmd/lumber/main.go` updates

- Added blank imports for `flyio` and `supabase` connectors (alongside existing `vercel`)
- Passes `cfg.Connector.Extra` through to `connector.ConnectorConfig` when building the pipeline config

### 5.3 — Config tests

4 tests in `internal/config/config_test.go`:

| Test | What it verifies |
|------|-----------------|
| `TestLoad_Defaults` | No env vars → provider defaults to `"vercel"`, Extra is `nil` |
| `TestLoad_ConnectorExtra` | `LUMBER_VERCEL_PROJECT_ID=proj_123` → `Extra["project_id"] == "proj_123"`, only 1 entry |
| `TestLoad_EmptyExtraOmitted` | Empty string env vars don't create entries, Extra stays `nil` |
| `TestLoad_MultipleProviders` | All 6 provider vars set simultaneously, all present in Extra with correct keys |

## Design Decisions

- **`nil` Extra when no vars set** — avoids allocating an empty map. Connectors already handle `nil` maps gracefully (map lookups on nil return zero value).
- **Flat Extra map shared across providers** — simpler than per-provider maps. Key names are unique across providers (`project_id` is only Vercel, `app_name` is only Fly.io, etc.). `poll_interval` is intentionally shared — it applies to whichever connector is active.
- **Table-driven env var mapping** — `loadConnectorExtra()` uses a slice of structs for the mapping, making it trivial to add new vars for future connectors.

## Files

| File | Action |
|------|--------|
| `internal/config/config.go` | Modified — added `Extra` field, `loadConnectorExtra()` |
| `internal/config/config_test.go` | Created |
| `cmd/lumber/main.go` | Modified — added flyio/supabase imports, pass Extra |

## Verification

```
$ go test ./internal/config/... -v -count=1
=== RUN   TestLoad_Defaults            --- PASS
=== RUN   TestLoad_ConnectorExtra      --- PASS
=== RUN   TestLoad_EmptyExtraOmitted   --- PASS
=== RUN   TestLoad_MultipleProviders   --- PASS
PASS

$ go build ./cmd/lumber  # compiles with all 3 connectors registered
```
