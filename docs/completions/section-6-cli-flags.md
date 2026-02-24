# Section 6: CLI Flags — Completion Notes

**Completed:** 2026-02-23
**Phase:** 5 (Pipeline Integration & Resilience)
**Depends on:** Sections 1-5 (uses slog, config validation, pipeline modes)

## Summary

Added CLI flags via stdlib `flag` package. Flags override env vars. Query mode is now accessible from the CLI — previously `main.go` always called `p.Stream()`, making `p.Query()` unreachable. `./lumber -help` shows all 8 flags.

## What Changed

### Modified Files

| File | Change | Why |
|------|--------|-----|
| `internal/config/config.go` | Added `Mode`, `QueryFrom`, `QueryTo`, `QueryLimit` fields to `Config`; added `LoadWithFlags()` function (~45 lines); added `"flag"` import; added Mode validation to `Validate()`; `Mode` read from `LUMBER_MODE` env var (default `"stream"`) | CLI flags + query mode config |
| `internal/config/config_test.go` | Added `Mode: "stream"` to `validConfig()` helper; added 5 new tests | Coverage for mode loading, validation, and both valid modes |
| `cmd/lumber/main.go` | Changed `config.Load()` to `config.LoadWithFlags()`; replaced single `p.Stream()` call with `switch cfg.Mode` dispatching to `p.Query()` or `p.Stream()` | Query mode now reachable from CLI |

### `LoadWithFlags()` Design

```go
func LoadWithFlags() Config {
    cfg := Load()                    // env vars first
    flag.String("mode", "", ...)     // define flags
    flag.Parse()                     // parse os.Args
    flag.Visit(func(f *flag.Flag) {  // overlay only explicitly-set flags
        switch f.Name { ... }
    })
    return cfg
}
```

Key: `flag.Visit()` iterates only flags that were explicitly set on the command line. This means:
- `-mode query` overrides `LUMBER_MODE=stream`
- Not passing `-verbosity` leaves the env var value intact
- `-pretty=false` is distinguishable from "not set" (unlike zero-value checking)

### CLI Flags

| Flag | Type | Description |
|------|------|-------------|
| `-mode` | string | Pipeline mode: stream or query (overrides `LUMBER_MODE`) |
| `-connector` | string | Connector provider (overrides `LUMBER_CONNECTOR`) |
| `-from` | string | Query start time, RFC3339 |
| `-to` | string | Query end time, RFC3339 |
| `-limit` | int | Query result limit |
| `-verbosity` | string | Output verbosity (overrides `LUMBER_VERBOSITY`) |
| `-pretty` | bool | Pretty-print JSON (overrides `LUMBER_OUTPUT_PRETTY`) |
| `-log-level` | string | Log level (overrides `LUMBER_LOG_LEVEL`) |

### Mode Switch (main.go)

Before:
```go
slog.Info("starting", "connector", cfg.Connector.Provider)
if err := p.Stream(ctx, connCfg); err != nil && err != context.Canceled {
    // ...
}
```

After:
```go
switch cfg.Mode {
case "query":
    slog.Info("starting query", ...)
    p.Query(ctx, connCfg, params)
default: // "stream"
    slog.Info("starting stream", ...)
    p.Stream(ctx, connCfg)
}
```

### Tests Added (5)

| Test | What it validates |
|------|-------------------|
| `TestLoad_ModeDefault` | Default mode is "stream" |
| `TestLoad_ModeEnv` | `LUMBER_MODE=query` sets mode |
| `TestValidate_BadMode` | `"replay"` → error containing "mode" |
| `TestValidate_StreamModeValid` | `"stream"` passes validation |
| `TestValidate_QueryModeValid` | `"query"` passes validation |

### `validConfig()` Update

Added `Mode: "stream"` to the helper since `Validate()` now rejects empty/invalid modes. Without this, all existing validation tests would fail.

## New Environment Variable

| Variable | Default | Type | Description |
|----------|---------|------|-------------|
| `LUMBER_MODE` | `stream` | string | Pipeline mode: stream or query |

## Verification

```
go test ./internal/config/...    # 30 tests pass
go build ./cmd/lumber            # compiles cleanly
./lumber -help                   # shows all 8 flags
go test ./...                    # full suite passes
```

### `-help` Output

```
Usage of ./lumber:
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
```
