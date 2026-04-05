# Phase 10, Section 5: Config Validation Fixes

**Completed:** 2026-04-05

**Scope:** Updated config defaults and validation to support local connectors (stdin, file) and the wizard trigger. Default connector changed from `"vercel"` to empty string, API key validation now skips local connectors, file connector gets path validation, and a new `-file` flag + `LUMBER_FILE_PATH` env var are wired in.

**Plan:** `docs/executing/phase-10-cli-wizard.md`, Section 5

---

## What Was Done

### 5a: Default Connector → Empty String

**File:** `internal/config/config.go:67`

```go
// Before:
Provider: getenv("LUMBER_CONNECTOR", "vercel"),

// After:
Provider: getenv("LUMBER_CONNECTOR", ""),
```

An empty provider signals "not configured" — this is the trigger for the wizard (Section 6). Previously, a bare `lumber` command defaulted to Vercel and immediately failed requiring an API key.

### 5b: Skip API Key Validation for Local Connectors

**File:** `internal/config/config.go:179-182`

```go
localConnectors := map[string]bool{"stdin": true, "file": true, "": true}
if c.Connector.Provider != "" && c.Connector.APIKey == "" && !localConnectors[c.Connector.Provider] {
    errs = append(errs, "LUMBER_API_KEY is required for cloud connectors")
}
```

`stdin` and `file` connectors don't need authentication. The empty provider is also excluded — it will be resolved by the wizard before validation runs.

### 5c: File Connector Path Validation

**File:** `internal/config/config.go:183-191`

When `provider == "file"`:
1. Checks `Extra["file"]` is non-empty → error: "file path is required for file connector"
2. Checks `os.Stat(filePath)` → error: "log file not found: <path>"

This catches misconfiguration early with actionable messages rather than letting the file connector fail at runtime with a generic "open: no such file" error.

### 5d: `-file` CLI Flag and `LUMBER_FILE_PATH` Env Var

**File:** `internal/config/config.go`

- New flag: `flag.String("file", "", "Log file path (for file connector)")`
- Flag visit handler: sets `cfg.Connector.Extra["file"]` (creates the map if nil)
- New env var: `LUMBER_FILE_PATH` → `Extra["file"]` in `loadConnectorExtra()`

### 5e: Updated Existing Tests + New Tests

**File:** `internal/config/config_test.go`

**Updated existing tests:**
- `TestLoad_Defaults` — expects `""` instead of `"vercel"` for default provider
- `TestLoad_ConnectorExtra`, `TestLoad_EmptyExtraOmitted`, `TestLoad_MultipleProviders` — added `LUMBER_FILE_PATH` to env var cleanup lists to prevent test interference

**7 new tests:**

| Test | What |
|------|------|
| `TestValidate_StdinSkipsAPIKey` | provider=stdin, no API key → passes |
| `TestValidate_FileConnectorRequiresFilePath` | provider=file, no path → "file path is required" |
| `TestValidate_FileConnectorValidatesFileExists` | provider=file, nonexistent path → "log file not found" |
| `TestValidate_FileConnectorValid` | provider=file, real temp file → passes |
| `TestValidate_CloudConnectorStillRequiresAPIKey` | provider=vercel, no API key → still errors |
| `TestValidate_EmptyProviderSkipsAPIKey` | provider="", no API key → passes |
| `TestLoad_FilePathEnv` | `LUMBER_FILE_PATH=/var/log/app.log` → `Extra["file"]` populated |

---

## Verification

- `go build ./...` — compiles cleanly
- `go test ./internal/config/` — 43/43 pass (36 existing + 7 new)
- `go test ./...` — full suite green (26 packages)

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/config/config.go` | modified | Default provider, API key validation, file validation, `-file` flag, `LUMBER_FILE_PATH` env var |
| `internal/config/config_test.go` | modified | Updated 4 existing tests, added 7 new tests |

**Modified files: 2. Total: 2.**

---

## Design Decisions

- **Map-based local connector check:** `localConnectors := map[string]bool{...}` is clearer and more extensible than `provider != "stdin" && provider != "file"`. Adding another local connector later is a one-line change.
- **File existence check in Validate(), not in the connector:** Catching a nonexistent file during config validation gives the user a clear error with the flag/env var name, before the pipeline starts. The connector's own error would be more generic.
- **`LUMBER_FILE_PATH` in `loadConnectorExtra()`:** Follows the same pattern as `LUMBER_FLY_APP_NAME` and `LUMBER_SUPABASE_PROJECT_REF` — provider-specific env vars map to `Extra` keys. This means the file connector reads it from the same place regardless of whether it was set via flag or env var.
