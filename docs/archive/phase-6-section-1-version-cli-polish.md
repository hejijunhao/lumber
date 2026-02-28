# Phase 6, Section 1: Version Flag & CLI Polish — Completion Notes

**Completed:** 2026-02-24
**Phase:** 6 (Beta Validation & Polish)
**Depends on:** Phase 5 complete (CLI flags infrastructure via `LoadWithFlags`)

## Summary

Added a `-version` flag that prints `lumber 0.5.0-beta` and exits, and customized `flag.Usage` so `-help` shows a banner with version, usage modes, all flags, and key environment variables. A first-time user running `lumber -help` now gets a complete orientation.

## What Changed

### Modified Files

| File | Lines changed | What |
|------|---------------|------|
| `internal/config/config.go` | +30 | `Version` const, `ShowVersion` field, `-version` flag, custom `flag.Usage` |
| `internal/config/config_test.go` | +10 | 2 new tests |
| `cmd/lumber/main.go` | +6 | `fmt` import, `ShowVersion` check before `logging.Init()` |

### `internal/config/config.go`

- **`Version` constant** — `"0.5.0-beta"` at package level. Exported so `main.go` can reference it in the version output.
- **`ShowVersion bool`** — new field on `Config`. Set by the `-version` flag in `LoadWithFlags()`.
- **`-version` flag** — registered alongside the other 8 flags in `LoadWithFlags()`.
- **Custom `flag.Usage`** — set before `flag.Parse()` so `-help` triggers it. Outputs:
  1. Banner line with version: `lumber 0.5.0-beta — log normalization pipeline`
  2. Usage section with two mode examples (stream default, query with time range)
  3. Standard flag listing via `flag.PrintDefaults()`
  4. Key environment variables with brief descriptions
  5. Pointer to README for full reference

### `cmd/lumber/main.go`

- **Version check before init** — `cfg.ShowVersion` is checked immediately after `LoadWithFlags()`, before `logging.Init()` and `Validate()`. This means `lumber -version` works without model files on disk or an API key configured.
- Added `"fmt"` to imports for `fmt.Fprintf`.

### Tests Added (2)

| Test | What it validates |
|------|-------------------|
| `TestLoad_ShowVersionDefault` | `Load()` returns `ShowVersion=false` (flag not parsed via `Load`, only via `LoadWithFlags`) |
| `TestVersion_IsSet` | `Version` constant is non-empty |

## Design Decisions

- **Version in config, not main** — `Version` lives in `internal/config` rather than `cmd/lumber` so it can be referenced from both the `-version` handler (main.go) and the `flag.Usage` banner (config.go) without circular dependencies.
- **`ShowVersion` as a Config field, not `os.Exit` in config** — keeps config as a pure data loader with no side effects. The exit happens in main.go, which is the natural place for process lifecycle decisions.
- **Version check before `logging.Init()`** — `lumber -version` should work in any state: no model files, no API key, no valid config. Checking it first avoids any initialization that could fail or produce output.
- **`flag.Usage` set in `LoadWithFlags()`, not main.go** — all flag registration happens in `LoadWithFlags()`, so the usage function naturally lives alongside the flag definitions. The usage text references the `Version` constant directly.
- **Subset of env vars in `-help`** — only the 5 most important env vars are shown (connector, API key, verbosity, dedup, log level). The full list (21 vars) would overwhelm the help output. The README is referenced for the complete table.

## Verification

```
go test ./internal/config/...                  # 27 tests pass (25 existing + 2 new)
go build -o bin/lumber ./cmd/lumber            # compiles
./bin/lumber -version                          # prints "lumber 0.5.0-beta"
./bin/lumber -help                             # shows banner, modes, flags, env vars
go vet ./cmd/lumber/... ./internal/config/...  # clean
go test ./...                                  # full suite passes
```

### `-version` output

```
lumber 0.5.0-beta
```

### `-help` output

```
lumber 0.5.0-beta — log normalization pipeline

Usage:
  lumber [flags]

Modes:
  lumber                              Stream logs (default)
  lumber -mode query -from T -to T    Query historical logs

Flags:
  -connector string
        Connector: vercel, flyio, supabase
  -from string
        Query start time (RFC3339)
  -limit int
        Query result limit
  -log-level string
        Log level: debug, info, warn, error
  -mode string
        Pipeline mode: stream or query
  -pretty
        Pretty-print JSON output
  -to string
        Query end time (RFC3339)
  -verbosity string
        Verbosity: minimal, standard, full
  -version
        Print version and exit

Environment variables:
  LUMBER_CONNECTOR      Log provider (vercel, flyio, supabase)
  LUMBER_API_KEY        Provider API key/token
  LUMBER_VERBOSITY      Output verbosity (minimal, standard, full)
  LUMBER_DEDUP_WINDOW   Dedup window duration (e.g. 5s, 0 to disable)
  LUMBER_LOG_LEVEL      Internal log level (debug, info, warn, error)

  See README for full configuration reference.
```
