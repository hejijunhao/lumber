# Phase 10, Sections 7, 9, 10, 11: Integration, Tests, Usage, Version

**Completed:** 2026-04-05

**Scope:** Wired the wizard into `main.go`, added wizard tests, updated CLI usage text, and bumped version to 0.10.0. These sections complete the Phase 10 implementation.

**Plan:** `docs/executing/phase-10-cli-wizard.md`, Sections 7, 9, 10, 11

---

## Section 7: Wizard Integration in main.go

**File:** `cmd/lumber/main.go`

### 7a: Detect "unconfigured" state

After `config.LoadWithFlags()` and before validation, the following decision tree runs when `cfg.Connector.Provider == ""`:

1. **TTY** → `cli.RunWizard(cfg)` launches the interactive wizard
2. **Not TTY, piped data** → auto-detect stdin connector (`cfg.Connector.Provider = "stdin"`)
3. **Not TTY, no pipe** → print usage hint listing available connector modes, exit 1

TTY detection uses `os.ModeCharDevice` on `os.Stdin.Stat()`, the standard Go idiom.

### 7b: Model readiness check (non-wizard path)

After the wizard block and before `cfg.Validate()`, `cli.ModelsReady(cfg)` checks all model files + ORT library. If missing:
- TTY: suggests running the wizard or `make download-model`
- Non-TTY: lists manual download commands and env var overrides

This replaces the previous failure mode (3 separate "file not found" errors from validation) with a single actionable message.

### 7c: Startup banner

```go
fmt.Fprintf(os.Stderr, "\n  lumber %s\n\n", config.Version)
```

Printed to stderr so it doesn't mix with NDJSON on stdout.

### 7d: New connector imports

Added blank imports for the new connectors:
```go
_ "github.com/kaminocorp/lumber/internal/connector/file"
_ "github.com/kaminocorp/lumber/internal/connector/stdin"
```

### 7e: Helper functions

- `isTerminal(f *os.File) bool` — checks `os.ModeCharDevice`
- `stdinHasData() bool` — checks if stdin is a pipe (not a char device)

---

## Section 9: Tests

**New file:** `internal/cli/wizard_test.go`

6 tests for the non-interactive parts of the wizard:

| Test | What |
|------|------|
| `TestModelsReady_AllPresent` | All model files + ORT library present → true |
| `TestModelsReady_MissingModel` | Model file missing → false |
| `TestModelsReady_AllMissing` | All files missing → false |
| `TestBuildSummary_StdoutOnly` | Summary with stdin/stream/standard shows correct text |
| `TestBuildSummary_WithFileAndWebhook` | Summary includes file path and webhook indicator |
| `TestBuildSummary_FileConnector` | Summary with file connector and full verbosity |

**Why not test the interactive forms?** `huh`'s programmatic input API (`WithProgramInput`) requires a bubbletea program runner and simulated key sequences. This couples tests to the form library's internal event model — brittle and low-value. The wizard's logic is straightforward: each form maps user input to config fields. The `ModelsReady` and `buildSummary` functions are the non-trivial logic worth testing.

---

## Section 10: Updated Usage Text

**File:** `internal/config/config.go`

New `flag.Usage` output:

```
lumber 0.10.0 — log normalization pipeline

Usage:
  lumber                                Interactive setup wizard
  lumber -connector stdin               Classify piped logs
  lumber -connector file -file PATH     Classify a log file
  lumber -connector vercel              Stream from Vercel (requires LUMBER_API_KEY)
  cat app.log | lumber                  Auto-detect piped input

Environment variables:
  LUMBER_CONNECTOR      Log provider (vercel, flyio, supabase, stdin, file)
  LUMBER_API_KEY        Provider API key/token (cloud connectors only)
  LUMBER_FILE_PATH      Log file path (file connector)
  ...
```

Changes from previous:
- Added wizard, stdin, file, and pipe auto-detect usage examples
- Added `stdin` and `file` to connector list
- API key now labeled "cloud connectors only"
- Added `LUMBER_FILE_PATH` env var

---

## Section 11: Version Bump

**File:** `internal/config/config.go`

```go
var Version = "0.10.0"
```

---

## Verification

- `go build ./...` — compiles cleanly
- `go test ./...` — full suite green (26 packages)
- `internal/cli` now has 6 tests (previously 0)

---

## Files Changed

| File | Action | Section |
|------|--------|---------|
| `cmd/lumber/main.go` | modified | 7 — wizard trigger, TTY detection, model check, banner, connector imports |
| `internal/cli/wizard_test.go` | **new** | 9 — 6 tests for ModelsReady + buildSummary |
| `internal/config/config.go` | modified | 10 — usage text; 11 — version bump to 0.10.0 |

**New files: 1. Modified files: 2. Total: 3.**

---

## Design Decisions

- **Model readiness check runs after wizard, before validation:** The wizard handles its own model download flow. The post-wizard `ModelsReady` check catches the non-wizard path (user set flags but forgot to download models). Ordering: wizard → model check → validate → pipeline.
- **`stdinHasData` vs `isTerminal` distinction:** `isTerminal` checks if stdin is a TTY (char device). `stdinHasData` checks if stdin is a pipe (not a char device). The auto-detect pattern `cat app.log | lumber` triggers when stdin is a pipe — we set provider to "stdin" automatically. This matches the behavior of tools like `jq` and `bat`.
- **Startup banner on stderr:** NDJSON output goes to stdout. Everything else (banner, wizard forms, logging) goes to stderr. This means `lumber -connector stdin < app.log > classified.ndjson` works cleanly — banner and progress on the terminal, classified output in the file.
- **Wizard tests focus on pure functions:** `ModelsReady` and `buildSummary` are the testable logic. The interactive form flow is tested manually via the verification checklist. This avoids brittle tests coupled to `huh`/`bubbletea` internal event mechanics.
