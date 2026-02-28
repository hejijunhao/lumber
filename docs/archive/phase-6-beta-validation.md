# Phase 6: Beta Validation & Polish — Implementation Plan

## Goal

Make Lumber ready to hand to someone else. The README accurately describes the tool. Error messages are clear. Edge cases are handled. The changelog is up to date. A new user can clone, build, configure, and run Lumber against a real log provider without hitting surprises.

**Success criteria:**
- README reflects the current 42-leaf taxonomy, all 3 connectors, CLI flags, query mode, dedup, all env vars
- `-version` flag prints version and exits
- UNCLASSIFIED events get a default severity instead of empty string
- Empty/whitespace-only logs return UNCLASSIFIED instead of a random low-confidence match
- Expanded real-world test corpus (~50 additional entries) covering production log patterns
- Changelog entry for Phase 5 (pipeline integration & resilience)
- `flag.Usage` customized with banner and usage examples
- `go vet ./...` and `go build ./cmd/lumber` clean, all tests pass
- A new user can follow the README from zero to running pipeline

---

## Current State

**Working:**
- Binary compiles, `-help` shows all 8 flags
- Config validation catches missing files, bad thresholds, invalid modes
- Structured logging via `log/slog` throughout
- Per-log error resilience, bounded dedup buffer, graceful shutdown
- 162 tests across 14 test packages, all passing
- `make build` produces `bin/lumber`, `make download-model` fetches ONNX files

**Needs attention:**
- README is significantly outdated:
  - Taxonomy table shows old labels (APPLICATION root, old SECURITY → now ACCESS, old REQUEST children)
  - Status checklist stale (shows tokenizer as incomplete)
  - Missing: CLI flags, query mode, dedup, structured logging, connectors beyond Vercel
  - Missing: env vars added in Phases 3–5 (`LUMBER_LOG_LEVEL`, `LUMBER_DEDUP_WINDOW`, `LUMBER_MODE`, etc.)
  - Project structure incomplete (missing `logging/`, `testdata/`)
- No `-version` flag
- UNCLASSIFIED events have empty Severity (known issue from Phase 3 notes)
- Empty/whitespace logs classify arbitrarily at ~0.6 confidence instead of returning UNCLASSIFIED
- No `flag.Usage` customization — default Go format with no banner or examples
- No changelog entry for Phase 5
- No real-world log patterns in the test corpus (all 104 entries are synthetic)

---

## Section 1: Version Flag & CLI Polish

**What:** Add a `-version` flag that prints version and exits. Customize `flag.Usage` with a banner and usage examples so `-help` is genuinely useful for a first-time user.

### Tasks

1.1 **Add version constant and `-version` flag** in `cmd/lumber/main.go`.

Add at package level:

```go
const version = "0.5.0-beta"
```

Add to `LoadWithFlags()` in `internal/config/config.go`:

```go
showVersion := flag.Bool("version", false, "Print version and exit")
```

Handle in `LoadWithFlags()` after `flag.Parse()`:

```go
if *showVersion {
    fmt.Fprintf(os.Stderr, "lumber %s\n", Version)
    os.Exit(0)
}
```

Since `LoadWithFlags()` owns all flag parsing, the version flag and exit live there. Export a `Version` constant from config so main.go can also reference it (e.g., in startup log).

Alternatively, to keep config focused on configuration and avoid `os.Exit` in a config function: add the flag in `main.go` before calling `LoadWithFlags()`. But since `LoadWithFlags()` calls `flag.Parse()`, and Go's `flag` package panics on double-parse, the flag must be registered before that call. The cleanest approach: export a `Version` var from config, register `-version` in `LoadWithFlags`, and return a `parsed` struct that includes a `ShowVersion bool` field. Main checks it after the call.

Simpler: just put the version flag and `flag.Usage` customization in `main.go`, and move `flag.Parse()` out of `LoadWithFlags()` so main.go controls the parse lifecycle. This is cleaner but changes the `LoadWithFlags` API.

**Recommended approach:** Keep `flag.Parse()` inside `LoadWithFlags()` (no API change). Add `-version` as a recognized flag. Add `ShowVersion bool` to `Config`. Main checks it immediately:

```go
cfg := config.LoadWithFlags()
if cfg.ShowVersion {
    fmt.Fprintf(os.Stderr, "lumber %s\n", config.Version)
    os.Exit(0)
}
```

1.2 **Customize `flag.Usage`** in `LoadWithFlags()` before `flag.Parse()`:

```go
flag.Usage = func() {
    fmt.Fprintf(os.Stderr, `lumber %s — log normalization pipeline

Usage:
  lumber [flags]

Modes:
  lumber                              Stream logs (default)
  lumber -mode query -from T -to T    Query historical logs

Flags:
`, Version)
    flag.PrintDefaults()
    fmt.Fprintf(os.Stderr, `
Environment variables:
  LUMBER_CONNECTOR    Log provider (vercel, flyio, supabase)
  LUMBER_API_KEY      Provider API key/token
  LUMBER_VERBOSITY    Output verbosity (minimal, standard, full)
  LUMBER_DEDUP_WINDOW Dedup window duration (e.g. 5s, 0 to disable)
  LUMBER_LOG_LEVEL    Internal log level (debug, info, warn, error)

  See README for full configuration reference.
`)
}
```

### Files

| File | Action |
|------|--------|
| `internal/config/config.go` | Add `Version` const, `ShowVersion` field, `-version` flag in `LoadWithFlags()`, custom `flag.Usage` |
| `internal/config/config_test.go` | Add version/showversion test |
| `cmd/lumber/main.go` | Check `cfg.ShowVersion`, exit with version string |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestLoad_ShowVersionDefault` | Default ShowVersion is false |

Note: `flag.Usage` customization is best verified manually via `./lumber -help`. The output is visual, not logic.

### Verification

```
go test ./internal/config/...
go build -o bin/lumber ./cmd/lumber
./bin/lumber -version    # prints "lumber 0.5.0-beta"
./bin/lumber -help       # shows banner, modes, flags, env vars
```

---

## Section 2: Edge Case Hardening

**What:** Fix two known classification edge cases from Phase 3's "Known limitations" list:
1. UNCLASSIFIED events have empty Severity — should default to "warning"
2. Empty/whitespace-only logs classify at ~0.6 confidence instead of returning UNCLASSIFIED

### Tasks

2.1 **Default severity for UNCLASSIFIED events** in `internal/engine/engine.go`.

In the `Process()` method, after classification, when the result is UNCLASSIFIED (empty type/category or below threshold), set severity to `"warning"`:

```go
if event.Type == "UNCLASSIFIED" && event.Severity == "" {
    event.Severity = "warning"
}
```

This is the right default: UNCLASSIFIED means the system couldn't determine what the log is. That's worth attention (not info-level silence) but not an error in the log source.

2.2 **Return UNCLASSIFIED for empty/whitespace-only input** in `internal/engine/engine.go`.

Add an early return in `Process()` before embedding:

```go
func (e *Engine) Process(raw model.RawLog) (model.CanonicalEvent, error) {
    cleaned := strings.TrimSpace(raw.Raw)
    if cleaned == "" {
        return model.CanonicalEvent{
            Type:       "UNCLASSIFIED",
            Category:   "empty_input",
            Severity:   "warning",
            Timestamp:  raw.Timestamp,
            Summary:    "",
            Confidence: 0,
            Raw:        raw.Raw,
        }, nil
    }
    // ... existing embed + classify path ...
}
```

This avoids wasting an embedding call on input that has no semantic content. The `[CLS][SEP]` embedding for empty text is not meaningless — it just matches a random category, which is worse than a clear UNCLASSIFIED.

2.3 **Add `"strings"` import** to `internal/engine/engine.go` if not already present.

### Files

| File | Action |
|------|--------|
| `internal/engine/engine.go` | Early return for empty input, default UNCLASSIFIED severity |
| `internal/engine/engine_test.go` | Update/add edge case tests |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestProcessEmptyLog_ReturnsUnclassified` | Empty string → UNCLASSIFIED with "warning" severity, zero confidence |
| `TestProcessWhitespaceLog_ReturnsUnclassified` | Whitespace-only → same UNCLASSIFIED behavior |
| `TestProcessUnclassifiedSeverity` | When classifier returns UNCLASSIFIED (low confidence), severity is "warning" not "" |

Note: The first two tests do NOT require ONNX — they return before the embedding call. The third may need the real engine if it relies on a low-confidence classification; alternatively, test it via the engine constructor with a mock embedder.

### Verification

```
go test ./internal/engine/...
go build ./cmd/lumber
```

---

## Section 3: Expanded Real-World Test Corpus

**What:** Add ~50 real-world-patterned log entries to the test corpus covering production log formats that the synthetic corpus doesn't represent. This validates that the taxonomy works on the messy, inconsistent logs Lumber will actually encounter.

### Tasks

3.1 **Add real-world entries to `internal/engine/testdata/corpus.json`**.

Target patterns not well-represented in the current 104-entry synthetic corpus:

| Category | Real-world patterns to add |
|----------|---------------------------|
| ERROR.connection_failure | AWS RDS connection reset, Redis ECONNRESET, MongoDB connection timeout |
| ERROR.runtime_exception | Python tracebacks (multi-line), Go panic with goroutine dump, Node.js unhandled rejection |
| ERROR.rate_limited | Stripe 429 response, GitHub API rate limit with reset header |
| REQUEST.success | Nginx access log format, Apache Combined Log Format, CloudFront log |
| REQUEST.client_error | 404 with full URL path, 422 with validation details |
| DEPLOY.build_failed | Webpack error output, Docker build failure, Go compilation error |
| SYSTEM.health_check | Kubernetes liveness probe, ELB health check, Consul health check |
| SYSTEM.process_lifecycle | Systemd start/stop, Docker container lifecycle, PM2 restart |
| ACCESS.login_success | OAuth callback success, SAML assertion, JWT issued |
| ACCESS.session_expired | JWT expired, session cookie invalid, refresh token revoked |
| PERFORMANCE.db_slow_query | PostgreSQL slow query log, MySQL slow query with EXPLAIN |
| PERFORMANCE.latency_spike | P99 latency alert, Datadog APM trace, New Relic alert |

Each entry follows the existing corpus format:

```json
{"raw": "...", "expected_type": "ERROR", "expected_category": "connection_failure", "expected_severity": "error"}
```

3.2 **Run accuracy test and tune taxonomy descriptions if needed**.

After adding entries, run `TestCorpusAccuracy` with the ONNX model. If any new entries misclassify, tune the relevant taxonomy descriptions in `internal/engine/taxonomy/default.go` — the same iterative process used in Phase 3 (round 1: identify confusion pairs, round 2: add discriminating keywords, round 3: verify).

**Target: 100% top-1 accuracy on expanded corpus.** This is achievable because:
- We control the corpus entries (can adjust genuinely ambiguous ones)
- Description tuning proved highly effective in Phase 3 (89% → 100% in 3 rounds)
- The new entries are deliberately chosen to be clearly classifiable

3.3 **Update `internal/engine/testdata/testdata_test.go`** — update corpus count assertion from 104 to new total.

### Files

| File | Action |
|------|--------|
| `internal/engine/testdata/corpus.json` | Add ~50 real-world-patterned entries |
| `internal/engine/taxonomy/default.go` | Tune descriptions if accuracy drops (likely minor) |
| `internal/engine/testdata/testdata_test.go` | Update count assertion |

### Tests

| Test | What it validates |
|------|-------------------|
| `TestCorpusAccuracy` (existing) | 100% top-1 accuracy on expanded corpus (requires ONNX) |
| `TestCorpusSeverityConsistency` (existing) | All correctly classified entries have correct severity |
| `TestLoadCorpus` (existing) | Corpus JSON parses, all leaves covered, severities valid |

### Verification

```
go test -v -run TestCorpus ./internal/engine/...    # requires ONNX model
go test ./internal/engine/testdata/...               # corpus validation (no ONNX)
```

---

## Section 4: README Overhaul

**What:** Rewrite the README to accurately reflect the current state. A new user should be able to go from zero to running pipeline by following the README alone.

### Tasks

4.1 **Update Taxonomy table** — replace the old 8-category table with the current 42-leaf taxonomy across 8 roots (ERROR, REQUEST, DEPLOY, SYSTEM, ACCESS, PERFORMANCE, DATA, SCHEDULED). Drop the APPLICATION root (removed in Phase 3). Show actual leaf labels, not the scaffolding-era ones.

4.2 **Update Configuration table** — add all env vars introduced in Phases 3–5:

| Variable | Default | Description |
|----------|---------|-------------|
| `LUMBER_CONNECTOR` | `vercel` | Log provider: vercel, flyio, supabase |
| `LUMBER_API_KEY` | — | Provider API key/token |
| `LUMBER_ENDPOINT` | — | Provider API endpoint URL override |
| `LUMBER_MODE` | `stream` | Pipeline mode: stream or query |
| `LUMBER_MODEL_PATH` | `models/model_quantized.onnx` | Path to ONNX model file |
| `LUMBER_VOCAB_PATH` | `models/vocab.txt` | Path to tokenizer vocabulary |
| `LUMBER_PROJECTION_PATH` | `models/2_Dense/model.safetensors` | Path to projection weights |
| `LUMBER_VERBOSITY` | `standard` | Output verbosity: minimal, standard, full |
| `LUMBER_OUTPUT` | `stdout` | Output destination |
| `LUMBER_OUTPUT_PRETTY` | `false` | Pretty-print JSON output |
| `LUMBER_CONFIDENCE_THRESHOLD` | `0.5` | Min confidence to classify (0–1) |
| `LUMBER_DEDUP_WINDOW` | `5s` | Dedup window duration (0 disables) |
| `LUMBER_MAX_BUFFER_SIZE` | `1000` | Max events buffered before force flush |
| `LUMBER_LOG_LEVEL` | `info` | Internal log level: debug, info, warn, error |
| `LUMBER_SHUTDOWN_TIMEOUT` | `10s` | Max drain time on shutdown |

Provider-specific:

| Variable | Provider | Description |
|----------|----------|-------------|
| `LUMBER_VERCEL_PROJECT_ID` | Vercel | Vercel project ID |
| `LUMBER_VERCEL_TEAM_ID` | Vercel | Vercel team ID (optional) |
| `LUMBER_FLY_APP_NAME` | Fly.io | Fly.io application name |
| `LUMBER_SUPABASE_PROJECT_REF` | Supabase | Supabase project reference |
| `LUMBER_SUPABASE_TABLES` | Supabase | Comma-separated table list |
| `LUMBER_POLL_INTERVAL` | All | Polling interval for stream mode |

4.3 **Add CLI Flags section** — document the 8 flags with examples:

```bash
# Stream from Fly.io with debug logging
lumber -connector flyio -log-level debug

# Query last hour of Vercel logs
lumber -mode query -connector vercel \
  -from 2026-02-24T00:00:00Z -to 2026-02-24T01:00:00Z

# Pretty-print with minimal verbosity
lumber -pretty -verbosity minimal
```

4.4 **Add Connectors section** — document the 3 implemented connectors (Vercel, Fly.io, Supabase) with brief setup instructions for each.

4.5 **Update Quickstart** — add query mode example, show connector selection, mention dedup.

4.6 **Update Project Structure** — add `logging/`, `testdata/`, `pipeline/`, connector sub-packages.

4.7 **Update Status checklist** — mark all completed phases, indicate beta status.

4.8 **Update Embedding Model section** — the current table says "Embedding dimension: 384" but the model outputs 384 from the transformer then projects to 1024 via the dense layer. Clarify: "Output dimension: 1024 (384-dim transformer + learned projection)".

### Files

| File | Action |
|------|--------|
| `README.md` | Full rewrite of Taxonomy, Configuration, Quickstart, Project Structure, Status; add CLI Flags and Connectors sections |

### Tests

No code tests — documentation only. Verified by reading.

### Verification

- Read through the README as a new user
- Verify every env var and flag matches `internal/config/config.go`
- Verify taxonomy table matches `internal/engine/taxonomy/default.go`
- Verify project structure matches actual directory layout

---

## Section 5: Changelog — Phase 5 Entry

**What:** Write the Phase 5 changelog entry covering pipeline integration & resilience (the 7 sections completed in the previous phase). This is the largest changelog entry yet — it touches 12 files with 7 distinct features.

### Tasks

5.1 **Add Phase 5 entry to `docs/changelog.md`** at the top (below the index).

Structure following existing entries:
- **Header:** `0.5.0 — 2026-02-23` (or current date)
- **Subtitle:** Pipeline integration & resilience
- **Summary paragraph:** what Phase 5 achieved
- **Added:** structured logging, config validation, per-log error handling, bounded dedup buffer, graceful shutdown, CLI flags, integration tests
- **Changed:** main.go rewritten, pipeline.go refactored
- **Design decisions:** key choices made
- **Known limitations:** what's deferred
- **Files changed:** all 12 files

5.2 **Update the index** at the top of changelog.md with the new entry.

### Files

| File | Action |
|------|--------|
| `docs/changelog.md` | Add 0.5.0 entry, update index |

### Tests

No code tests — documentation only.

### Verification

- Verify file counts and test counts match actual `git diff` from Phase 5 commit
- Cross-reference with `docs/completions/section-*.md` files for accuracy

---

## Section 6: Final Verification & Cleanup

**What:** Run the full verification suite, fix any issues found, and ensure the project is clean for beta.

### Tasks

6.1 **Run full test suite:**

```bash
go test ./...                                         # all non-ONNX tests pass
go test -v -run Integration ./internal/pipeline/...   # ONNX integration tests (if model available)
go test -v -run TestCorpus ./internal/engine/...      # corpus accuracy (if model available)
```

6.2 **Run static analysis:**

```bash
go vet ./...
go build ./cmd/lumber
```

6.3 **Verify binary behavior:**

```bash
./bin/lumber -version     # prints version
./bin/lumber -help        # shows banner, modes, flags, env vars
./bin/lumber              # exits with clear error if no API key / model files
```

6.4 **Audit unused code** — check for any dead code, unused imports, or unreferenced types introduced during the 5-phase build. Remove if found.

6.5 **Cross-check README against code:**
- Every env var in README exists in `config.go`
- Every CLI flag in README exists in `LoadWithFlags()`
- Taxonomy table matches `taxonomy/default.go`
- Project structure matches actual `ls` output

6.6 **Verify `make` targets:**

```bash
make clean
make build         # produces bin/lumber
make test          # all tests pass
```

### Files

| File | Action |
|------|--------|
| Various | Fix any issues found during verification |

### Tests

No new tests — this section validates existing tests.

### Verification

All commands in 6.1–6.6 pass cleanly.

---

## Implementation Order

```
Section 1: Version Flag & CLI Polish (no dependencies)
    │
    ├──→ Section 2: Edge Case Hardening (no dependencies, parallel with 1)
    │       │
    │       └──→ Section 3: Expanded Corpus (depends on 2 — empty-input handling affects corpus entries)
    │
    ├──→ Section 4: README Overhaul (depends on 1 — needs to document -version flag)
    │
    ├──→ Section 5: Changelog (no dependencies, parallel with 1-4)
    │
    └──→ Section 6: Final Verification (depends on all above)
```

Recommended sequence: **1 → 2 → 3 → 4 → 5 → 6**

Sections 1 and 2 are independent and can be done in parallel. Section 4 should come after 1 (to document the version flag) and ideally after 2 (to note the edge case fixes). Section 5 is independent but benefits from being done after 1–4 so the Phase 5 entry can reference the complete state.

---

## Files Summary

| File | Sections | Action |
|------|----------|--------|
| `internal/config/config.go` | 1 | Add `Version` const, `ShowVersion` field/flag, custom `flag.Usage` |
| `internal/config/config_test.go` | 1 | Add ShowVersion default test |
| `cmd/lumber/main.go` | 1 | Check ShowVersion, print version and exit |
| `internal/engine/engine.go` | 2 | Early return for empty input, default UNCLASSIFIED severity |
| `internal/engine/engine_test.go` | 2 | Add/update edge case tests |
| `internal/engine/testdata/corpus.json` | 3 | Add ~50 real-world-patterned entries |
| `internal/engine/testdata/testdata_test.go` | 3 | Update corpus count assertion |
| `internal/engine/taxonomy/default.go` | 3 | Tune descriptions if needed |
| `README.md` | 4 | Full content update |
| `docs/changelog.md` | 5 | Add Phase 5 entry, update index |

**New files: 0. Modified files: 10.**
**Estimated new tests: ~4** (1 config + 3 edge case)

---

## What's Explicitly Not In Scope

- **Additional connectors** (AWS, Datadog, Grafana Loki) — beta validates the pattern with 3 connectors; others follow the same interface
- **gRPC/WebSocket/webhook output** — stdout JSON is sufficient for beta
- **Adaptive taxonomy** (self-growing/trimming) — requires production log volume over time
- **Field extraction** (regex parsing of service names, hosts, IPs) — valuable but not core to classification
- **Docker image** — post-beta distribution concern
- **Multi-tenancy** — single-instance-per-source for beta
- **Library API** (public `pkg/` surface) — everything stays in `internal/`
- **Configuration files** (YAML/TOML) — env vars + CLI flags for beta
- **Metrics/Prometheus** — structured logging provides observability
- **Sustained live traffic testing** — deferred to post-beta operational validation (the expanded corpus provides offline validation; live testing requires provider credentials and is an operational concern, not a code concern)
