# Phase 10, Section 1: Add `charmbracelet/huh` Dependency

**Completed:** 2026-04-05

**Scope:** Introduce the Charm ecosystem as Lumber's first interactive-UI dependency. This provides the building blocks for the CLI setup wizard (Section 6) and styled terminal output (Section 8).

**Plan:** `docs/executing/phase-10-cli-wizard.md`, Section 1

---

## What Was Done

### 1. Added `charmbracelet/lipgloss` as a Direct Dependency

**Command:** `go get github.com/charmbracelet/huh@latest && go mod tidy`

`huh` v1.0.0 and its full transitive dependency tree were fetched. However, Go modules are import-driven — `go mod tidy` prunes anything not reachable from an actual `import` statement. Since no Go file imports `huh` yet (that happens in Section 6), `huh` itself was pruned.

To anchor the Charm ecosystem in `go.mod` now (rather than deferring entirely to Section 6), we created `internal/cli/style.go` (planned for Section 8) which imports `lipgloss` directly. This keeps `lipgloss` and its transitive dependencies in `go.mod` immediately.

**Result in `go.mod`:**

| Dependency | Status | Why |
|------------|--------|-----|
| `github.com/charmbracelet/lipgloss v1.1.0` | direct | Imported by `internal/cli/style.go` |
| `github.com/charmbracelet/colorprofile` | indirect | Transitive via lipgloss |
| `github.com/charmbracelet/x/ansi` | indirect | Transitive via lipgloss |
| `github.com/charmbracelet/x/cellbuf` | indirect | Transitive via lipgloss |
| `github.com/charmbracelet/x/term` | indirect | Transitive via lipgloss |

`huh`, `bubbletea`, and `bubbles` will appear when `internal/cli/wizard.go` is written in Section 6 and imports `github.com/charmbracelet/huh`.

### 2. Created `internal/cli/style.go`

**New file:** `internal/cli/style.go`

Defines three reusable `lipgloss` styles for the wizard UI:

```go
titleStyle   — Bold, purple (color 99). Used for the wizard header.
successStyle — Green (color 42). Used for "✓ Models ready" and post-wizard confirmation.
mutedStyle   — Gray (color 241). Used for secondary text like "No connector configured."
```

This file was pulled forward from Section 8 because it's a leaf dependency (no imports from other new code) and provides the import anchor for `lipgloss`.

### 3. Created `internal/cli/` Package

**New directory:** `internal/cli/`

This package didn't exist before. It will house:
- `style.go` (this section) — lipgloss styles
- `wizard.go` (Section 6) — interactive wizard logic
- `wizard_test.go` (Section 9) — wizard tests

---

## Verification

- `go build ./...` — compiles cleanly with no errors
- `go mod tidy` — no unused dependencies remain
- `lipgloss` and transitive deps present in `go.mod`

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `go.mod` | modified | Added lipgloss + Charm transitive deps |
| `go.sum` | modified | Checksums for all new dependencies |
| `internal/cli/style.go` | **new** | Lipgloss style definitions (pulled forward from Section 8) |

**New files: 1. Modified files: 2. Total: 3.**

---

## Design Decisions

- **Why create style.go now instead of a dummy import file:** The plan calls for `style.go` in Section 8 anyway, and it's a standalone file with no dependencies on other new code. Using the real file as the import anchor avoids throwaway placeholder code.
- **Why lipgloss sticks but huh doesn't:** Go's module system is strictly import-driven. `go mod tidy` removes any dependency not reachable from a source-level `import`. Since `style.go` imports `lipgloss` but nothing imports `huh` yet, only `lipgloss` persists. This is correct — `huh` arrives in Section 6 when `wizard.go` imports it.
- **Section 8 partially complete:** `style.go` is done. The `printHeader()` and `printReady()` helper functions from Section 8 will be added in Section 6 or 8, as they depend on `fmt`/`os` wiring that makes more sense alongside the wizard.
