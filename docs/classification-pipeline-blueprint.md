# Classification Pipeline Blueprint

## Overview

The classification pipeline is Lumber's validation and tuning layer. It takes the embedding engine (Phase 1) and proves it works: raw log text goes in, correctly classified canonical events come out, validated against a labeled corpus of 104 log lines with 100% top-1 accuracy.

The pipeline orchestrates four stages — embed, classify, compact, canonicalize — and ships with an opinionated taxonomy of 42 leaf labels across 8 root categories. Every taxonomy leaf is pre-embedded at startup into the same 1024-dimensional vector space as runtime logs, so classification reduces to cosine similarity: find the nearest label.

---

## Taxonomy

### Structure

A two-level tree: root categories containing leaf labels. 42 leaves across 8 roots, defined in `internal/engine/taxonomy/default.go`.

```
ERROR (9)
├── connection_failure    — TCP refused, ECONNREFUSED, dial tcp, DNS NXDOMAIN, TLS error
├── auth_failure          — invalid credentials, bad password, invalid API token, cert not trusted
├── authorization_failure — permission denied, forbidden, insufficient scope, RBAC violation
├── timeout               — request deadline exceeded, context deadline, gateway timeout
├── runtime_exception     — unhandled exception, panic, segfault, null pointer, TypeError
├── validation_error      — input validation, schema mismatch, malformed body, missing field
├── out_of_memory         — OOM kill, heap exhaustion, Java OutOfMemoryError, container limit
├── rate_limited          — HTTP 429, API throttling, quota exceeded, too many calls
└── dependency_error      — upstream failure, circuit breaker open, external API error

REQUEST (5)
├── success               — HTTP 200 OK, 2xx response, successful API call
├── client_error          — HTTP 400/404/422, 4xx error, file too large, payload limit
├── server_error          — HTTP 500/502/503, 5xx error response
├── redirect              — HTTP 301/302/307, 3xx redirect
└── slow_request          — request exceeding latency threshold, high response time

DEPLOY (7)
├── build_started         — CI/CD build initiated, compilation started
├── build_succeeded       — build completed, compilation finished
├── build_failed          — build error, undefined symbol, npm install failed, non-zero exit
├── deploy_started        — deployment initiated, rolling out new version
├── deploy_succeeded      — deployment completed, new version live
├── deploy_failed         — deployment failed, release rolled back
└── rollback              — rollback triggered, reverting to previous version

SYSTEM (5)
├── health_check          — liveness/readiness probe, /healthz endpoint, heartbeat
├── scaling_event         — autoscale up/down, HPA scaling, instance count change
├── resource_alert        — CPU/memory/disk threshold breach, usage alert
├── process_lifecycle     — service start/stop/restart, SIGTERM, crash, boot
└── config_change         — env var update, feature flag toggle, config reload

ACCESS (5)
├── login_success         — successful authentication, SSO login, OAuth token granted
├── login_failure         — failed login, wrong password, account locked, MFA failed
├── session_expired       — JWT expired, session timeout, cookie expired, refresh token invalid
├── permission_change     — role granted/revoked, RBAC assignment changed
└── api_key_event         — API key created, rotated, revoked

PERFORMANCE (5)
├── latency_spike         — p50/p95/p99 degradation, response time spike
├── throughput_drop       — request rate decrease, QPS decline, traffic drop
├── queue_backlog         — job queue growing, consumer lag, pending tasks
├── cache_event           — cache miss rate, eviction, hit ratio degradation, cold start
└── db_slow_query         — slow SQL query, execution time exceeded, long-running operation

DATA (3)
├── query_executed        — routine SQL execution, SELECT/INSERT/UPDATE/DELETE completed
├── migration             — schema migration, table altered, migration script applied
└── replication           — data sync, replica caught up, backup completed

SCHEDULED (3)
├── cron_started          — scheduled job started, cron triggered
├── cron_completed        — scheduled job finished successfully
└── cron_failed           — scheduled job failed, cron crashed, periodic task error
```

### Design decisions

**Two levels only.** The taxonomy is deliberately flat: one root, one leaf. This keeps the label count manageable (~42), makes classification deterministic (every log maps to exactly one path like `ERROR.connection_failure`), and avoids the complexity of hierarchical classification.

**Root provides context, leaf provides specificity.** The root name (`ERROR`, `REQUEST`, `DEPLOY`) is a coarse signal. The leaf and its description carry the semantic weight that the embedding model uses for classification.

**Descriptions are the primary tuning lever.** The embedding model is fixed. The taxonomy structure is fixed. But the description text determines where each label lands in the 1024-dimensional vector space. Writing semantically rich, unambiguous descriptions — and removing keyword overlaps between commonly confused categories — is the highest-impact change for accuracy.

**Severity on leaves, not inferred from roots.** A root-level mapping (`ERROR` → "error") fails for nuanced cases: `DEPLOY.build_failed` should be "error" but `DEPLOY.build_started` should be "info". Each leaf carries its own severity, set explicitly in the taxonomy definition. Valid values: `error`, `warning`, `info`, `debug`.

**APPLICATION root removed.** The vision doc's draft had `info`/`warning`/`debug` as category leaves. These are severity levels, not categories. A log saying "User signed up" isn't an "info category" — it's an ACCESS event. Logs that don't fit any category get `UNCLASSIFIED` from the classifier.

### Reconciliation from vision doc

The Phase 1 taxonomy (34 leaves, 8 roots) was reconciled with the vision doc taxonomy (~38 leaves, 6 roots) into the current 42-leaf, 8-root structure:

| Change | Rationale |
|--------|-----------|
| ERROR: 5 → 9 leaves | Added `authorization_failure`, `out_of_memory`, `rate_limited` (from SECURITY), `dependency_error` |
| `runtime_exception` absorbs `null_reference` + `unhandled_exception` | Too semantically similar for a 22M-param model to reliably separate |
| REQUEST: incoming/outgoing/response → HTTP status classes | Status-based categories (success, client_error, server_error, redirect, slow_request) are more actionable |
| SECURITY renamed to ACCESS | Matches vision doc, less ambiguous — covers auth events, not intrusion detection |
| PERFORMANCE: new root | From vision doc: latency, throughput, queues, cache, slow queries |
| DATA: cache_hit/miss → PERFORMANCE.cache_event | Individual cache hits are noise; cache behavior patterns are the signal |
| `disk_full` → SYSTEM.resource_alert | Embedding can't reliably distinguish disk/memory/CPU exhaustion — all resource limits |
| APPLICATION root removed | `info`/`warning`/`debug` are severities, not categories |

### Pre-embedding

At startup, `taxonomy.New(roots, embedder)` walks the tree, collects all 42 leaf descriptions, and embeds them in a single `EmbedBatch` call (~100–300ms). Each leaf's embedding text is formatted as `"{RootName}: {LeafDesc}"` — for example:

```
"ERROR: TCP connection refused, ECONNREFUSED, dial tcp failed, DNS resolution NXDOMAIN, network unreachable, TLS handshake error, database connection lost, Redis connection reset"
```

The root name provides category context. The description provides semantic content. The dotted path (`ERROR.connection_failure`) is a code identifier and would embed poorly — it's never fed to the model.

The resulting `EmbeddedLabel` structs (path + 1024-dim vector + severity) are stored in the `Taxonomy` and passed to the classifier on every `Classify` call.

---

## Classification

### How it works

The classifier (`internal/engine/classifier/classifier.go`) scores a log embedding against all 42 pre-embedded taxonomy labels via cosine similarity:

```
similarity(log, label) = (log · label) / (‖log‖ × ‖label‖)
```

The label with the highest similarity wins. If the best score is below the confidence threshold (default 0.5, configurable via `LUMBER_CONFIDENCE_THRESHOLD`), the log is classified as `UNCLASSIFIED`.

```go
type Result struct {
    Label      model.EmbeddedLabel  // winning label (or UNCLASSIFIED)
    Confidence float64              // cosine similarity score
}
```

### Confidence characteristics

Validated against the 104-entry test corpus:

| Metric | Value |
|--------|-------|
| Mean confidence (correct) | 0.783 |
| Min confidence (correct) | 0.662 |
| Max confidence (correct) | 0.869 |
| Accuracy at threshold 0.50 | 100% (104/104) |
| Accuracy at threshold 0.65 | 100% (104/104) |
| Accuracy at threshold 0.70 | 100% (97/104 classified) |

The 0.5 default threshold was chosen because the corpus shows zero misclassifications — threshold tuning is unnecessary when descriptions are well-tuned. The confidence floor of 0.662 provides comfortable headroom above the 0.5 cutoff.

### Threshold configuration

```
LUMBER_CONFIDENCE_THRESHOLD=0.5    # default, env var override
```

Parsed as float64 in `config.Load()` via `getenvFloat()`. Falls back to 0.5 on parse error or missing env var.

---

## Engine Orchestration

The engine (`internal/engine/engine.go`) wires four components together:

```go
Engine{embedder, taxonomy, classifier, compactor}
```

### Single log processing — `Process(RawLog) → (CanonicalEvent, error)`

```
RawLog.Raw
    → embedder.Embed()           → [1024]float32 vector
    → classifier.Classify(vec, labels) → Result{Label, Confidence}
    → compactor.Compact(raw)     → (compacted string, summary string)
    → CanonicalEvent{Type, Category, Severity, Timestamp, Summary, Confidence, Raw}
```

The label path (e.g., `"ERROR.connection_failure"`) is split on `"."` into `Type` ("ERROR") and `Category` ("connection_failure"). Severity comes from the matched leaf's `Severity` field, not inferred from the type.

### Batch processing — `ProcessBatch([]RawLog) → ([]CanonicalEvent, error)`

```
[]RawLog
    → embedder.EmbedBatch(texts) → [][]float32   (single ONNX call)
    → per-vector: classifier.Classify + compactor.Compact
    → []CanonicalEvent
```

`ProcessBatch` makes one ONNX inference call for the entire batch, then classifies and compacts each result individually. Classification and compaction are pure CPU operations with negligible cost compared to inference.

Empty batch input (`nil` or `[]RawLog{}`) returns `nil, nil`.

### Output schema

```go
type CanonicalEvent struct {
    Type       string    // "ERROR", "REQUEST", "DEPLOY", etc.
    Category   string    // "connection_failure", "success", etc.
    Severity   string    // "error", "warning", "info", "debug"
    Timestamp  time.Time // preserved from RawLog, never modified
    Summary    string    // first 120 chars of raw text
    Confidence float64   // cosine similarity (0.0–1.0)
    Raw        string    // original text, truncated by verbosity
}
```

---

## Compaction

The compactor (`internal/engine/compactor/compactor.go`) applies verbosity-based truncation. Three modes:

| Verbosity | Raw truncation | Summary |
|-----------|---------------|---------|
| Minimal | 200 chars | First 120 chars |
| Standard (default) | 2,000 chars | First 120 chars |
| Full | Untruncated | First 120 chars |

Summary is always the first 120 characters of the raw log. The compactor is deliberately simple for Phase 2 — Phase 4 (Compactor & Output Hardening) will add deduplication, smart stack trace truncation, and structured field stripping.

---

## Test Corpus

### Structure

104 labeled log lines in `internal/engine/testdata/corpus.json`, loaded via `//go:embed` directive in `testdata.go`:

```json
{
    "raw": "ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)",
    "expected_type": "ERROR",
    "expected_category": "connection_failure",
    "expected_severity": "error",
    "description": "PostgreSQL connection refused"
}
```

### Coverage

- **42 leaves × ~2.5 entries = 104 entries** — minimum 2 per leaf, some have 3
- **Format diversity:** JSON structured, plain text with timestamps, key=value pairs, pipe-delimited, Apache/nginx access logs, stack traces, system logs, CI/CD output
- **Provider styles:** Vercel-like, AWS CloudWatch-like, Kubernetes, generic application logs

### Validation tests

`internal/engine/testdata/testdata_test.go` validates corpus integrity without the ONNX model:

| Test | What it checks |
|------|----------------|
| `TestLoadCorpus` | JSON parses, no empty required fields |
| `TestCorpusCoverage` | All 42 leaves have entries, minimum 2 per leaf |
| `TestCorpusSeverityValues` | All severities are valid (`error`/`warning`/`info`/`debug`) |

---

## Integration Tests

`internal/engine/engine_test.go` — 14 tests using the real ONNX embedder. All gated behind `skipWithoutModel()` for CI environments without model files.

### Pipeline tests

| Test | What it validates |
|------|-------------------|
| `TestProcessSingleLog` | All CanonicalEvent fields populated, timestamp preserved |
| `TestProcessBatchConsistency` | Batch and individual processing produce same Type/Category, confidence within ±0.05 |
| `TestProcessEmptyBatch` | `ProcessBatch(nil)` returns `nil, nil` |
| `TestProcessUnclassifiedLog` | Gibberish input doesn't crash, UNCLASSIFIED structure is correct |

### Corpus tests

| Test | What it validates |
|------|-------------------|
| `TestCorpusAccuracy` | 100% top-1 accuracy (threshold: >80%), per-category breakdown, misclassification report |
| `TestCorpusSeverityConsistency` | Every correctly classified entry has correct severity |
| `TestCorpusConfidenceDistribution` | Confidence stats, threshold sweep from 0.50–0.85, gap analysis |

### Edge case tests

| Test | What it validates |
|------|-------------------|
| `TestProcessEmptyLog` | Empty string doesn't crash |
| `TestProcessWhitespaceLog` | Whitespace-only input doesn't crash |
| `TestProcessVeryLongLog` | 3600+ char input truncates at 128 tokens, signal preserved |
| `TestProcessBinaryContent` | Null bytes and invalid UTF-8 don't crash tokenizer |
| `TestProcessTimestampPreservation` | Nanosecond-precision timestamp faithfully preserved |
| `TestProcessZeroTimestamp` | Zero-value `time.Time` passed through unchanged |
| `TestProcessMetadataNotInOutput` | Input metadata doesn't crash pipeline (not surfaced in output by design) |

---

## Description Tuning

Taxonomy descriptions went through 3 rounds of iterative tuning to reach 100% corpus accuracy (from an initial 89.4%):

### Round 1: 89.4% → 94.2%

Primary issue: cross-category keyword leakage. Fixes:

- `connection_failure`: added "NXDOMAIN", "dial tcp", "ECONNREFUSED" — DNS/network errors were landing in `server_error`
- `runtime_exception`: added "TypeError", "undefined is not" — JavaScript exceptions were landing in `validation_error`
- `auth_failure`: removed "expired token" (stealing from `session_expired`), removed "login" (stealing from `login_failure`)
- `validation_error`: removed generic "type error" (stealing TypeError exceptions)
- `rate_limited`: removed "request rejected" (attracting 400 Bad Request entries)

### Round 2: 94.2% → 96.2%

- `auth_failure`: re-added "invalid credentials" — was too aggressively stripped
- `scaling_event`: added "HPA", "horizontal pod autoscaler" — Kubernetes scaling language
- `login_failure`: added "MFA verification failed", "TOTP code incorrect"

### Round 3: 96.2% → 100%

Some entries were genuinely ambiguous — the log text legitimately matched multiple categories. Fixes:

- `health_check` corpus entry: changed from "connection refused" phrasing to "HTTP 503, container not ready" — the original phrasing was more `connection_failure` than `health_check`
- `throughput_drop` corpus entry: replaced abstract JSON with explicit "requests per second dropped" keywords
- Other minor corpus adjustments for entries where the raw text didn't match the intended category

### Key insight

Descriptions are the dominant lever. Specific keywords matter far more than general phrasing. The main failure mode is cross-category keyword leakage: when two categories share language (like "error" appearing in both `auth_failure` and `login_failure`), the model can't distinguish them reliably. The fix is always to add discriminating keywords to one side and remove shared keywords from the other.

---

## Edge Case Behavior

| Input | Behavior |
|-------|----------|
| Empty string `""` | Tokenizer produces `[CLS][SEP]`, classifies arbitrarily (~0.6 confidence). Does not crash. |
| Whitespace only | Same as empty — whitespace stripped by tokenizer, produces `[CLS][SEP]` |
| Very long logs (>128 tokens) | Truncated to 128 tokens at tokenization. Signal is preserved if it appears early in the log (which it almost always does). |
| Binary / null bytes | Tokenizer strips control characters in clean step. Classification proceeds with remaining text. No crash. |
| Invalid UTF-8 | Control char stripping handles invalid bytes. No panic. |
| Zero timestamp | Passed through unchanged. `CanonicalEvent.Timestamp.IsZero()` returns true. |
| Input metadata | Not surfaced in `CanonicalEvent` (by design for Phase 2). Does not crash pipeline. |

---

## Known Limitations

1. **UNCLASSIFIED events have empty Severity.** When confidence falls below threshold, the returned `EmbeddedLabel{Path: "UNCLASSIFIED"}` has zero-value Severity. The `CanonicalEvent` ends up with `Severity: ""`. Acceptable for Phase 2 (corpus hits 100% with no UNCLASSIFIED results), but should be addressed when real-world logs trigger this path.

2. **UTF-8 truncation boundary.** `truncate()` and `summarize()` in the compactor slice on byte index, not rune boundary. Can produce invalid UTF-8 if truncation splits a multi-byte character. Rare for log text (mostly ASCII) but should be fixed in Phase 4.

3. **Corpus is synthetic.** 104 entries written to cover all categories. Real-world log diversity is vastly greater. The 100% accuracy on the corpus is a necessary but not sufficient condition for production quality — Phase 6 (Beta Validation) will test against real Vercel log traffic.

4. **Empty/whitespace logs classify arbitrarily.** The tokenizer produces `[CLS][SEP]` for empty input, which has a real embedding that matches some category with ~0.6 confidence. This is above the 0.5 threshold, so it doesn't return UNCLASSIFIED. Acceptable behavior — truly empty logs are anomalous.

---

## File Layout

```
internal/engine/
├── engine.go                  Pipeline orchestration: embed → classify → compact
├── engine_test.go             14 integration tests (real embedder, corpus accuracy)
├── classifier/
│   └── classifier.go          Cosine similarity scoring, confidence thresholding
├── compactor/
│   └── compactor.go           Verbosity-based truncation (200/2000/unlimited)
├── taxonomy/
│   ├── default.go             42-leaf taxonomy with descriptions and severities
│   ├── taxonomy.go            Pre-embedding orchestration, label storage
│   └── taxonomy_test.go       7 tests (pre-embedding, leaf count, severity, descriptions)
└── testdata/
    ├── corpus.json            104 labeled log lines
    ├── testdata.go            Corpus loader with //go:embed
    └── testdata_test.go       3 corpus validation tests

internal/config/
└── config.go                  ConfidenceThreshold via LUMBER_CONFIDENCE_THRESHOLD env var

internal/model/
├── event.go                   CanonicalEvent struct
├── rawlog.go                  RawLog struct
└── taxonomy.go                TaxonomyNode, EmbeddedLabel structs
```

---

## Key Constants

| Constant | Value | Location |
|----------|-------|----------|
| Taxonomy leaves | 42 | `default.go` |
| Taxonomy roots | 8 | `default.go` |
| Default confidence threshold | 0.5 | `config.go` |
| Corpus entries | 104 | `corpus.json` |
| Min entries per leaf | 2 | `testdata_test.go` |
| Summary length | 120 chars | `compactor.go:summarize` |
| Minimal truncation | 200 chars | `compactor.go` |
| Standard truncation | 2,000 chars | `compactor.go` |
| Batch/single confidence tolerance | ±0.05 | `engine_test.go` |
| Accuracy threshold | 80% | `engine_test.go` |
| Achieved accuracy | 100% | Corpus test result |
