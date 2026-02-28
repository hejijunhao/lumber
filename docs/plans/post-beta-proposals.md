# Lumber — Post-Beta Proposals

**Starting point:** v0.5.1-beta. Phases 1–6 complete. The pipeline works end-to-end: connectors ingest from Vercel/Fly.io/Supabase, the ONNX-based classification engine classifies against 42 taxonomy labels at 100% corpus accuracy, the compactor reduces token footprint, and stdout NDJSON output is production-quality. Structured logging, config validation, graceful shutdown, CLI flags, and a 153-entry test corpus are in place.

**This document proposes what to build next**, organized into three horizons: near-term hardening, medium-term capability expansion, and longer-term architectural evolution.

---

## Horizon 1: Hardening & Depth (what exists, made better)

These build on existing code with minimal new architecture. Each is independently shippable.

### 1.1 Field Extraction

**The gap:** Classification tells you *what kind* of event a log is (e.g., `ERROR.connection_failure`), but doesn't extract *structured fields* from the text — the host, port, service name, user ID, status code, duration, etc. that are buried in unstructured log lines. This is [Vision Open Question #3](../vision.md) and the most impactful single feature missing from the classification engine.

**Why it matters:** Downstream consumers (especially LLM agents) need structured fields to reason about logs. "Connection refused" is useful; "connection refused to `db-primary:5432` from `payments-service`" is actionable.

**Approach options:**

| Approach | Pros | Cons |
|----------|------|------|
| **A. Regex/heuristic per taxonomy category** | Fast, deterministic, no new deps | Brittle across formats, ongoing maintenance |
| **B. Named-entity-style extraction patterns** | Covers common patterns (IPs, hostnames, ports, HTTP codes, durations, paths) across all categories | Doesn't capture semantic role (is `5432` a port or an error code?) |
| **C. Category-aware regex bank** | Each taxonomy leaf gets specific extraction rules tuned to what that category typically contains | Best precision, but most labor-intensive to author |

**Recommendation:** Start with **B** (generic patterns) as a base layer, then layer **C** (category-specific rules) on top for the highest-value categories (connection_failure, request success/error, slow_request, db_slow_query). This is how most log parsers work — generic patterns catch the obvious, category-specific rules add precision.

**Output:** New optional fields on `CanonicalEvent`:

```go
type CanonicalEvent struct {
    // ... existing fields ...
    Fields map[string]string `json:"fields,omitempty"` // extracted structured fields
}
```

**Scope:** New package `internal/engine/extractor/`, wired into `engine.Process()` after classification. Category-aware: extraction rules are keyed by `Type.Category`. Corpus expanded with expected field extractions for validation.

---

### 1.2 Additional Stack Trace Detection

**The gap:** The compactor detects and truncates Java (`at ...`) and Go (`.go:\d+`, `goroutine`) stack traces, but not Python (`File "...", line N, in <module>`), Node.js (`at Object.<anonymous> (/path/file.js:N:N)`), or Rust (`thread 'main' panicked at ...`).

**Why it matters:** The 153-entry corpus already includes Python tracebacks. Without detection, these pass through untruncated at Standard/Minimal verbosity, defeating the compactor's purpose.

**Scope:** Add three regex patterns to `compactor.go`'s stack trace detector. Extend the corpus with representative traces for each. Small, self-contained change — likely < 100 lines.

---

### 1.3 Live Traffic Validation

**The gap:** The test corpus is comprehensive (153 entries, all 42 leaves) but entirely synthetic. Classification has never been validated against live production logs. The Phase 6 plan explicitly deferred this.

**Why it matters:** Synthetic logs are crafted to be classifiable. Real logs are messy — partial lines, interleaved multiline output, encoding issues, provider-specific metadata noise. Until Lumber classifies real traffic correctly, 100% corpus accuracy is a necessary but insufficient signal.

**Approach:**
1. Point Lumber at a real Vercel project with meaningful traffic for 1+ hour
2. Capture all raw → canonical pairs
3. Human-label a sample (200+ logs) and measure actual accuracy
4. Identify misclassification patterns → tune taxonomy descriptions or add corpus entries
5. Test with Fly.io and Supabase traffic for cross-provider validation

**Deliverable:** A documented accuracy report and any taxonomy/corpus fixes that result.

---

### 1.4 Performance Benchmarks

**The gap:** The vision doc targets "~5-10ms per log line." No benchmarks exist. We don't know actual throughput, latency percentiles, or memory usage under load.

**Approach:** Add Go benchmarks (`*testing.B`) for:
- `Embed()` single text (p50/p95/p99 latency)
- `EmbedBatch()` at various batch sizes (throughput curve)
- `engine.Process()` end-to-end (embed + classify + compact)
- `pipeline.Stream()` sustained throughput (logs/sec)
- Memory profiling under sustained load (detect leaks)

**Deliverable:** `internal/engine/bench_test.go` + documented results in a benchmark report. These become the baseline for future optimization work.

---

### 1.5 Nested JSON Field Stripping

**The gap:** The compactor's field stripper only handles top-level JSON keys. Nested objects (common in structured log formats) aren't traversed.

**Example:** `{"request": {"headers": {"x-request-id": "abc123", "x-trace-id": "def456"}, "body": "..."}}` — the high-cardinality fields survive because they're nested.

**Scope:** Extend `stripFields` in `compactor.go` to walk nested objects. Configurable depth limit to avoid pathological cases. Moderate change — the JSON parse/rebuild logic is already there.

---

## Horizon 2: Capability Expansion (new features, same architecture)

These extend Lumber's reach without changing the core pipeline architecture.

### 2.1 Output Backends

**The gap:** Only `stdout.Output` exists. The vision doc lists four output modes; three are unbuilt. The `Output` interface is ready — this is pure implementation.

**Priority order by value:**

| Backend | Why | Scope |
|---------|-----|-------|
| **File output** | Batch processing, log rotation, integration with existing tooling | `internal/output/file/` — NDJSON to a configurable path, optional rotation by size/time |
| **Webhook push** | Integration with Slack, PagerDuty, custom alert pipelines | `internal/output/webhook/` — HTTP POST of batched events, configurable URL + headers + batch size |
| **HTTP Query API** | Serve classified events to dashboards, agents, or other services on demand | `cmd/lumberd/` or a `-serve` flag — exposes a REST endpoint over classified events. Requires an in-memory or on-disk event store (significant new scope). |
| **gRPC/WebSocket streaming** | Real-time consumption by remote clients | Largest scope. Requires protocol definitions, connection management, backpressure. |

**Recommendation:** File output first (simplest, immediately useful for batch workflows), webhook second (enables alerting integrations), HTTP server later.

---

### 2.2 Additional Connectors

**The gap:** Three of the six providers mentioned in the vision doc are implemented. AWS CloudWatch, Datadog, and Grafana Loki are not.

**The existing pattern is strong:** shared httpclient, `init()` registration, httptest fixtures. Each new connector follows the same template.

| Connector | API Style | Complexity | Notes |
|-----------|-----------|------------|-------|
| **AWS CloudWatch** | SDK-based (FilterLogEvents, paginated) | Medium | Requires AWS SDK as new dep, IAM auth, log group/stream discovery |
| **Datadog** | REST + proprietary query language | Medium | Cursor-based pagination, API key + app key auth |
| **Grafana Loki** | HTTP + LogQL | Low–Medium | Standard REST, query language is well-documented |
| **Generic webhook receiver** | Inbound HTTP | Low | Lumber listens for POSTed logs — inverts the connector model (push, not pull) |
| **stdin** | Pipe/redirect | Very low | `cat logs.txt | lumber` — trivial but high-utility for one-off analysis |

**Recommendation:** `stdin` connector first (trivially simple, enormous utility for testing and one-off analysis — pipe any log file through Lumber). Then Grafana Loki or a generic webhook receiver.

---

### 2.3 Multi-Provider Fan-In

**The gap:** The pipeline currently wires one connector at a time. The vision mentions "concurrent multi-provider ingestion" via goroutines. A real deployment might need Vercel + Supabase logs flowing through the same Lumber instance.

**Approach:** A fan-in layer that launches multiple `connector.Stream()` goroutines, each writing to a shared channel that feeds the engine. Each `RawLog` carries its `Source` field, so downstream classification and output already handle mixed sources.

**Scope:** Primarily `cmd/lumber/main.go` changes — multi-connector config (comma-separated `-connector vercel,supabase` or multiple env vars), launch goroutines, merge channels. The pipeline itself is source-agnostic.

---

### 2.4 Configuration Files

**The gap:** All config is env vars + CLI flags. This works for simple deployments but becomes unwieldy with provider-specific settings, multiple connectors, and custom taxonomy overrides.

**Approach:** Support an optional `lumber.yaml` (or `lumber.toml`) that maps to the existing `Config` struct. Precedence: CLI flags > env vars > config file > defaults. Use a lightweight YAML parser (stdlib has none; `gopkg.in/yaml.v3` is the standard choice and would be the 3rd external dependency).

**Example:**

```yaml
mode: stream
verbosity: standard
confidence_threshold: 0.5

connectors:
  - provider: vercel
    api_key: ${LUMBER_API_KEY}  # env var interpolation
    project_id: prj_xxx
  - provider: supabase
    api_key: ${LUMBER_SUPABASE_KEY}
    project_ref: abc123

output:
  type: file
  path: /var/log/lumber/events.jsonl

logging:
  level: info
```

---

### 2.5 Push-Based Ingestion

**The gap:** All three connectors are poll-based. Fly.io actually supports NATS-based live tail; Vercel supports log drains (webhook push). Poll-based ingestion adds latency (5–10s default intervals) and wastes API quota on quiet periods.

**Approach:** Two new connector variants:
- **Webhook receiver** — Lumber runs an HTTP server, providers push to it. Works with Vercel log drains, custom integrations, and any provider supporting webhook delivery.
- **NATS subscriber** — for Fly.io's native streaming protocol. Adds `nats.go` as a dependency.

The `Connector` interface already supports this — `Stream()` returns a channel. Whether that channel is fed by polling or by a push listener is an implementation detail.

---

## Horizon 3: Architectural Evolution (new capabilities, new architecture)

These are larger efforts that introduce new architectural patterns.

### 3.1 Adaptive Taxonomy

**The gap:** The vision doc describes a self-growing/trimming taxonomy in detail. It's the most ambitious unbuilt feature — and potentially the most differentiating.

**How it works (from the vision doc):**
- **Growing:** Cluster low-confidence logs by embedding similarity → propose new labels
- **Trimming:** Prune labels that haven't matched above threshold for a configurable period
- **Splitting:** When a label catches too many semantically diverse logs, split by intra-cluster variance
- **Merging:** When sibling labels consistently match the same logs, merge them

**Prerequisites:** This requires significant log volume and observation time to be meaningful. It also requires persistent state (which logs matched which labels, confidence distributions over time, cluster analysis results).

**Approach:**
1. Start with **observation only**: track match counts, mean confidence, and embedding variance per label over time. Log these as structured metrics. No taxonomy mutations yet.
2. Add **suggestion mode**: when heuristics detect grow/trim/split/merge candidates, emit them as structured suggestions (log or file) for human review.
3. Eventually, add **auto-apply mode**: apply mutations automatically with rollback capability.

**Scope:** New package `internal/engine/taxonomy/adaptive/`. Requires a persistence layer (at minimum, a JSON file on disk for label statistics). This is a multi-phase effort.

---

### 3.2 HTTP Server Mode

**The gap:** Lumber is currently a CLI pipeline — it starts, processes, and exits (query mode) or runs indefinitely (stream mode). There's no way for external clients to query Lumber on demand.

**What this enables:**
- An LLM agent sends a request: "classify these 50 log lines" → Lumber responds with 50 canonical events
- A dashboard polls Lumber for recently classified events
- A webhook receiver that accepts log pushes and returns classified events synchronously

**Approach:** A new binary or mode (`lumber -mode serve` or `cmd/lumberd/`) that runs an HTTP server with:
- `POST /classify` — accept raw log text(s), return canonical event(s)
- `GET /events` — query recently classified events (requires an event store)
- `GET /taxonomy` — return the current taxonomy tree
- `GET /health` — liveness/readiness probe

**Scope:** This is significant new architecture. The `/classify` endpoint is straightforward (the engine already supports batch processing). The `/events` endpoint requires a storage layer (in-memory ring buffer, SQLite, or similar). Best split into phases.

---

### 3.3 Public Library API

**The gap:** Everything is in `internal/`. Lumber can only be used as a standalone binary. The vision mentions "Go module (importable as a library)" as a distribution target.

**What this enables:** Other Go programs embed Lumber's classification engine directly — no subprocess, no HTTP, no serialization overhead.

**Approach:** A thin `pkg/lumber/` surface that exposes:

```go
// pkg/lumber/lumber.go
type Lumber struct { ... }

func New(opts ...Option) (*Lumber, error)
func (l *Lumber) Classify(text string) (CanonicalEvent, error)
func (l *Lumber) ClassifyBatch(texts []string) ([]CanonicalEvent, error)
func (l *Lumber) Close() error
```

Internally wraps the engine. The `internal/` packages remain internal — `pkg/lumber` is the stable public contract.

**When:** After the internal API surface stabilizes. Exposing a public API too early creates backwards-compatibility obligations.

---

### 3.4 Distribution & Packaging

**The gap:** No Docker image, no release binaries, no Homebrew formula.

| Target | Scope | Notes |
|--------|-------|-------|
| **Docker image** | Dockerfile + CI build | Multi-stage build: Go compile + ONNX runtime + model files. ~150MB image. |
| **Release binaries** | GoReleaser or similar | Cross-compile for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64. Include model files or download on first run. |
| **Homebrew** | Formula in a tap repo | `brew install crimson-sun/tap/lumber` |

**Consideration:** Model files (~55MB) complicate distribution. Options: bundle in binary (large), download on first run (requires network), or ship separately (user manages).

---

## Proposed Phase Ordering

```
Horizon 1 (hardening)              Horizon 2 (expansion)              Horizon 3 (evolution)
─────────────────────              ─────────────────────              ─────────────────────

Phase 7: Extraction & Polish       Phase 9: Output & Connectors       Phase 11: Adaptive Taxonomy
 ├─ 1.1 Field extraction            ├─ 2.1 File + webhook output       ├─ 3.1 Observation mode
 ├─ 1.2 Stack trace detection       ├─ 2.2 stdin connector             ├─ 3.1 Suggestion mode
 └─ 1.5 Nested JSON stripping       ├─ 2.2 Loki connector             └─ 3.1 Auto-apply mode
                                     └─ 2.3 Multi-provider fan-in
Phase 8: Validation & Perf                                             Phase 12: Server & Library
 ├─ 1.3 Live traffic validation    Phase 10: Config & Push              ├─ 3.2 HTTP server mode
 └─ 1.4 Performance benchmarks     ├─ 2.4 Config files                 ├─ 3.3 Public library API
                                    └─ 2.5 Webhook receiver             └─ 3.4 Docker + releases
```

**Phases 7 and 8 can run in parallel** — extraction/polish is code work, validation/benchmarks is measurement work. Neither blocks the other.

**Phase 9 is the highest-value expansion** — file output and stdin connector are both low-effort, high-utility features that dramatically expand how Lumber can be used.

---

## Open Design Questions

These should be resolved before or during the relevant phase:

1. **Field extraction schema** — should extracted fields be a flat `map[string]string`, or typed (e.g., `Port int`, `Host string`)? Flat is simpler; typed enables downstream validation but requires a schema per category.

2. **stdin connector semantics** — one log per line? Or support multiline (e.g., stack traces)? If multiline, what's the delimiter? Blank line? Timestamp regex at line start?

3. **Event persistence for HTTP server** — in-memory ring buffer (simple, loses data on restart), SQLite (durable, adds a dependency), or external (Redis, Postgres — complicates deployment)?

4. **Model file distribution** — bundle in Docker image? Download on first run with `lumber init`? Ship as a separate artifact? Each has UX and size trade-offs.

5. **Multi-connector config UX** — comma-separated flag (`-connector vercel,supabase`)? Repeated flags (`-connector vercel -connector supabase`)? Config-file-only? The env var model (`LUMBER_CONNECTOR=vercel`) doesn't naturally extend to multiple providers.

6. **Public API stability** — when is the internal API stable enough to expose as `pkg/lumber`? What's the versioning strategy (semver, with the understanding that pre-1.0 allows breaking changes)?
