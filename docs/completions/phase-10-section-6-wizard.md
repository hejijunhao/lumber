# Phase 10, Section 6: Wizard Implementation

**Completed:** 2026-04-05

**Scope:** Implemented the interactive CLI setup wizard — a 4-form guided flow that takes users from zero configuration to a running pipeline. Covers model readiness checks with auto-download, source selection (local file/stdin or cloud provider with credentials), output destination configuration, and a summary confirmation screen.

**Plan:** `docs/executing/phase-10-cli-wizard.md`, Section 6

---

## What Was Done

### 1. Created `internal/cli/wizard.go`

**Entry point:** `RunWizard(base config.Config) (config.Config, error)`

Takes the current (incomplete) config, guides the user through setup, and returns a fully populated config struct. The caller (`main.go`) proceeds with normal validation + pipeline startup.

#### Form 1: Model Readiness

`ModelsReady(cfg)` checks existence of all 3 model files + ORT library (in model dir or cache dir). If missing:
- Prompts user via `huh.NewConfirm` to download (~65MB total)
- On confirm: downloads to `DefaultCacheDir()`, updates config paths to point at cache
- On decline: prints manual download instructions, returns error
- If models already present: skips silently

#### Form 2: Source Selection

Two-tier selection:
- **Local** → sub-select: file (prompts for path with existence validation) or stdin (prints "waiting for piped input" hint)
- **Cloud** → provider select (Vercel/Fly.io/Supabase) → masked API key input → provider-specific extras:
  - Vercel: optional project ID + team ID
  - Fly.io: required app name
  - Supabase: required project ref

Local sources always set `Mode = "stream"`.

#### Form 3: Output Options

- Multi-select for additional outputs (file, webhook) — stdout is always on
- Verbosity select (standard/minimal/full)
- If file selected: prompt for output path (defaults to `lumber-output.ndjson`)
- If webhook selected: prompt for URL with `http://`/`https://` validation
- If cloud source: mode select (stream/query), with RFC3339 time range prompts for query mode

#### Form 4: Summary Confirmation

`buildSummary()` renders a compact overview:
```
  Source:     vercel
  Mode:       stream
  Verbosity:  standard
  Output:     stdout + file (output.ndjson) + webhook
```

User confirms with "Start" or cancels. Cancel returns an error, main.go exits cleanly.

**Post-wizard:** sets `Output.Pretty = true` (TTY sessions get readable JSON), calls `printReady()` to show a success indicator.

### 2. Updated `internal/cli/style.go`

Added `printHeader()` and `printReady()` functions (completing Section 8 from the plan):

- `printHeader(version)` — styled banner + "No connector configured. Let's set up."
- `printReady(provider, mode)` — "✓ vercel → stream" confirmation line

### 3. `huh` Now a Direct Dependency

`go.mod` now includes `github.com/charmbracelet/huh v1.0.0` as a direct dependency (previously only `lipgloss` was anchored). Transitive deps now in `go.mod`:

| Package | Status |
|---------|--------|
| `charmbracelet/huh` | direct |
| `charmbracelet/lipgloss` | direct |
| `charmbracelet/bubbletea` | indirect |
| `charmbracelet/bubbles` | indirect |
| ~10 other charm/utility packages | indirect |

---

## Verification

- `go build ./...` — compiles cleanly
- `go test ./...` — full suite green (26 packages)
- `huh v1.0.0` confirmed in `go.mod` as direct dependency

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/cli/wizard.go` | **new** | 4-form interactive wizard (~330 lines) |
| `internal/cli/style.go` | modified | Added `printHeader()`, `printReady()` helpers |
| `go.mod` | modified | `huh v1.0.0` + transitive deps |
| `go.sum` | modified | Checksums for new deps |

**New files: 1. Modified files: 3. Total: 4.**

---

## Design Decisions

- **One `huh.NewForm().Run()` per logical step, not one giant form:** Each form (model check, source, output, summary) runs independently. This enables conditional flow — cloud users see different forms than local users, query mode shows time range prompts, etc. A single form can't branch.
- **`ModelsReady()` is exported:** Exported as `cli.ModelsReady(cfg)` because `main.go` (Section 7) will also call it for the non-wizard path to provide actionable "models not found" messages.
- **Pretty-print default for wizard sessions:** When the wizard runs (which means the user is in an interactive TTY), `Output.Pretty = true` is the right default. Users exploring Lumber for the first time benefit from readable JSON. Production usage (flags/env vars, no wizard) retains `Pretty = false`.
- **Provider-specific prompts in separate functions:** `promptVercelExtras`, `promptFlyioExtras`, `promptSupabaseExtras` are isolated so adding a new provider is a self-contained function + case statement entry.
- **Graceful cancellation:** Every `form.Run()` error is wrapped as "wizard cancelled" — whether the user pressed Ctrl+C or Escape. The caller gets a clear signal to exit without a stack trace.
- **Output file default:** If the user selects file output but submits an empty path, it defaults to `"lumber-output.ndjson"` rather than erroring. This follows the principle of sensible defaults for first-time users.

---

## Section 8 Status

Section 8 (Welcome Header & Styled Output) is now **fully complete** — `style.go` has the 3 lipgloss styles, `printHeader()`, and `printReady()`. No further work needed for Section 8.
