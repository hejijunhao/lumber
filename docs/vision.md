# Lumber — Vision

## What It Is

A high-performance, open-source log normalization pipeline written in Go.

Raw logs go in — from any provider, any application, any format. Structured, canonical, token-efficient events come out. What consumes the output (LLM agents, dashboards, alerting systems, humans) is not Lumber's concern.

## The Problem

### Provider fragmentation

Every log provider exposes a different API, auth mechanism, and response format:

- Vercel returns JSON arrays via REST with project-scoped tokens
- AWS CloudWatch uses paginated log groups/streams with IAM auth
- Fly.io exposes NATS-based live tail
- Datadog has its own query language and cursor-based pagination
- Grafana Loki uses LogQL over HTTP

These all represent the same fundamental thing — timestamped events from running software — but look completely different.

### Application-level chaos

Even within a single provider, every application logs differently:

```
App A:  {"level":"error","msg":"connection timeout","service":"payments","trace_id":"abc123","ts":1708300800}
App B:  ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)
App C:  2026-02-19T12:00:00Z | ERR | request failed | {"status":503,"path":"/api/checkout","duration_ms":30000}
App D:  payments.timeout db-primary:5432 30s
```

These all describe connection failures to a database. But they share no structure, no field names, no severity format, no timestamp format. The inconsistency isn't just between providers — it's between teams, services, and even endpoints within the same application.

### Why this matters now

Every AI agent startup is connecting LLMs to logs. They're doing it by dumping raw log text into context windows. This is:

- **Token-wasteful** — raw logs are verbose, redundant, full of metadata the LLM doesn't need
- **Unreliable** — inconsistent formats across sources mean the LLM reasons differently about equivalent events
- **Slow** — large payloads, no pre-processing, repeated parsing work on every request
- **Fragile** — each new log source requires custom integration code

## The Solution

A standalone pipeline that solves both layers of the problem:

```
Raw logs (any provider, any app format)
   ↓
PROVIDER CONNECTORS — unified ingestion from Vercel, AWS, Fly.io, Datadog, etc.
   ↓
EMBEDDING + TAXONOMY CLASSIFIER — semantic classification via local embedding model
   ↓
Structured canonical events (compact, queryable, consumer-agnostic)
```

---

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                    CONNECTORS                         │
│                                                       │
│   Vercel │ AWS │ Fly.io │ Datadog │ Grafana │ ...    │
│                                                       │
│   Each handles: auth, pagination, rate limiting,      │
│   streaming protocol, raw log retrieval               │
└──────────────────────┬───────────────────────────────┘
                       │
                       ▼  RawLog
┌──────────────────────────────────────────────────────┐
│               CLASSIFICATION ENGINE                   │
│                                                       │
│   1. Embed — log line → vector (local model)          │
│   2. Classify — cosine similarity against taxonomy    │
│   3. Canonicalize — map to uniform schema             │
│   4. Compact — strip noise, optimize for tokens       │
└──────────────────────┬───────────────────────────────┘
                       │
                       ▼  CanonicalEvent
┌──────────────────────────────────────────────────────┐
│                     OUTPUT                            │
│                                                       │
│   Stream (gRPC/WebSocket)  │  Query (HTTP API)        │
│   Webhook push             │  File export             │
└──────────────────────────────────────────────────────┘
```

### Layer 1: Connectors

Thin adapters, one per log provider. Each implements a common interface:

```go
type Connector interface {
    Stream(ctx context.Context, config ConnectorConfig) (<-chan RawLog, error)
    Query(ctx context.Context, config ConnectorConfig, params QueryParams) ([]RawLog, error)
}
```

- `Stream` — continuous live tail, long-running
- `Query` — on-demand historical fetch, bounded

Connectors do no classification. They output raw log entries with provider metadata attached.

### Layer 2: Classification Engine (Core Value)

This is where Lumber earns its place. The classification engine uses a **local embedding model** and an **opinionated taxonomy** to semantically classify every log line — regardless of format, structure, or origin.

#### How it works

**Step 1: Embed**
Each raw log line is passed through a small, local text embedding model. The log becomes a vector. This runs on-device — no external API calls, no network latency.

**Step 2: Classify against taxonomy**
Lumber ships with a pre-defined taxonomy of log categories, organized as a tree. Each leaf label in the taxonomy is pre-embedded (computed once at startup). Classification is a cosine similarity comparison between the log vector and all taxonomy label vectors. Top match wins.

```
Incoming log:  "ERROR [2026-02-19] UserService — connection refused (host=db-primary)"
   ↓ embed
Log vector:    [0.12, -0.45, 0.78, ...]
   ↓ cosine similarity against taxonomy
Best match:    ERROR → connection_failure  (score: 0.91)
```

This is the same approach as CLIP/SigLIP for images — embed both sides into a shared vector space, compare with cosine similarity — but applied to text-to-text matching.

**Step 3: Canonicalize**
Map the classified log into a uniform canonical event:

```json
{
  "type": "ERROR",
  "category": "connection_failure",
  "severity": "error",
  "timestamp": "2026-02-19T12:00:00Z",
  "summary": "UserService — connection refused (host=db-primary)",
  "confidence": 0.91,
  "raw": "ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)"
}
```

**Step 4: Compact**
Optimize output for token efficiency:
- Strip redundant metadata
- Truncate verbose fields (stack traces, request bodies) intelligently
- Deduplicate repeated events into counted summaries
- Configurable verbosity levels (minimal / standard / full)

#### The Taxonomy

An opinionated, curated tree of log categories. This is the ontology that all logs are classified into.

```
ERROR
├── connection_failure       — TCP/connection refused/timeout to dependencies
├── authentication_failure   — login failures, invalid tokens, expired sessions
├── authorization_failure    — permission denied, forbidden, insufficient scope
├── timeout                  — request/query/operation timeouts
├── null_reference           — null pointer, undefined, missing field errors
├── unhandled_exception      — uncaught throws, panics, segfaults
├── validation_error         — bad input, schema mismatch, type errors
├── rate_limited             — 429s, throttling, quota exceeded
├── out_of_memory            — OOM kills, heap exhaustion, allocation failures
├── disk_full                — storage capacity, write failures
└── dependency_error         — upstream/downstream service failures

REQUEST
├── success                  — 2xx responses, completed operations
├── client_error             — 4xx responses (non-auth)
├── server_error             — 5xx responses
├── redirect                 — 3xx responses
└── slow_request             — requests exceeding latency thresholds

DEPLOY
├── build_started
├── build_succeeded
├── build_failed
├── deploy_started
├── deploy_succeeded
├── deploy_failed
└── rollback

SYSTEM
├── health_check             — liveness/readiness probes
├── scaling_event            — autoscale up/down, instance changes
├── resource_alert           — CPU/memory/disk threshold breaches
├── process_lifecycle        — start, stop, restart, crash
└── config_change            — env var updates, feature flag toggles

ACCESS
├── login_success
├── login_failure
├── session_expired
├── permission_change
└── api_key_event            — creation, rotation, revocation

PERFORMANCE
├── latency_spike            — p50/p95/p99 degradation
├── throughput_drop          — request rate decrease
├── queue_backlog            — job/message queue growth
├── cache_miss_spike         — cache hit ratio degradation
└── db_slow_query            — query execution time anomalies
```

The taxonomy is opinionated by design. A finite, curated set of ~40-50 labels forces every log into a known category, making downstream consumption predictable. Labels can be added, removed, or reorganized — when the taxonomy changes, labels are simply re-embedded and classification continues.

#### Adaptive Taxonomy (Self-Growing/Trimming)

The taxonomy is not static. Lumber refines it over time based on actual log traffic:

**Growing:** When logs consistently land in a broad parent category with low confidence (e.g., many diverse logs hitting `ERROR` but not matching any child well), this signals the need for a new subcategory. Lumber clusters the low-confidence logs by embedding similarity, identifies coherent groups, and proposes new taxonomy labels.

**Trimming:** When a taxonomy label hasn't matched any logs above the confidence threshold for a configurable period, it's a candidate for pruning. Unused branches get removed to keep the tree lean.

**Splitting:** When a single label catches too many semantically diverse logs (high match count but high intra-cluster variance), it's split into more specific subcategories based on embedding clusters within the matched set.

**Merging:** When two sibling labels consistently match the same logs (high overlap in matched sets), they're merged.

This runs as a background process, not in the hot path. The runtime taxonomy at any given moment is fixed and deterministic — adaptations are applied as discrete updates.

#### Embedding Model

The engine runs a small, local text embedding model. No external API calls. No cloud dependency. Must run comfortably on minimal hardware (8GB MacBook Air).

**Leading candidates (as of Feb 2026):**

| Model | Params | Size (approx) | Notes |
|-------|--------|---------------|-------|
| MongoDB LEAF (mdbr-leaf-ir) | 23M | ~50MB | #1 on MTEB BEIR for models <100M params. Apache 2.0. CPU-only. |
| Snowflake Arctic Embed S | 22M | ~50MB | Strong retrieval accuracy approaching 100M-class models |
| all-MiniLM-L6-v2 | 22M | ~80MB | Battle-tested classic, fast, English-only |
| EmbeddingGemma-300M | ~300M | ~200MB (quantized) | Google. 100+ languages. <22ms on EdgeTPU |
| GTE-small | ~33M | ~70MB | Alibaba. Good quality-to-size ratio |
| Nomic Embed Text v2 | 305M (MoE) | ~400MB | Multilingual. Open source. Matryoshka dimensions |

For Lumber v1, a model in the 22-33M parameter range (LEAF, Arctic S, or GTE-small) is the right starting point — tiny, fast, CPU-friendly, good enough for log classification. Larger models can be swapped in later via the same interface.

The model runs via ONNX Runtime (Go bindings available via `onnxruntime-go`). No Python dependency.

### Layer 3: Output Interface

Consumers access canonical events through:
- **Streaming** — gRPC or WebSocket for real-time consumption
- **Query API** — HTTP for historical/on-demand access with filtering
- **Push** — webhook delivery to external systems
- **Export** — file-based output for batch processing

---

## Modes of Operation

### Live mode
Connectors stream logs continuously. The classification engine processes in real-time. Consumers subscribe to a stream of canonical events.

Primary use case: agent monitoring — an LLM agent receives a continuous feed of structured events and reacts to issues as they appear.

### Query mode
On-demand historical access. Consumer requests logs for a time range with optional filters. The engine fetches, classifies, and returns.

Primary use case: agent investigation — after detecting an issue, the agent queries for related logs in a specific window.

Both modes produce identical canonical output — same schema, same classification, same taxonomy.

---

## Design Principles

### Deterministic runtime
The hot path is fully deterministic. The embedding model produces the same vector for the same input. Cosine similarity is a pure function. The taxonomy is fixed at any given moment. Given the same raw log and the same taxonomy state, Lumber always produces the same canonical output.

### Small canonical ontology
A finite, curated set of ~40-50 leaf labels. This constraint forces classification to be meaningful rather than pass-through. It keeps downstream consumption predictable and reduces ambiguity for agent reasoning.

### Consumer-agnostic
Lumber does not know or care what consumes its output. LLM agent, dashboard, alerting system, data warehouse — all receive the same structured canonical events.

### Token-aware but not token-exclusive
Token efficiency is a first-class design goal (configurable compaction, intelligent truncation, deduplication) but the output format is useful for any consumer, not just LLMs.

### Zero external dependencies at runtime
No cloud API calls in the processing path. The embedding model runs locally. Classification is on-device. Lumber works fully offline after initial setup.

---

## Technology

**Language:** Go
- Goroutines for concurrent multi-provider ingestion
- Channels for internal pipeline stages
- Strong standard library for HTTP, gRPC, regex
- Single binary deployment
- Fast compilation for rapid iteration

**Embedding runtime:** ONNX Runtime via `onnxruntime-go`
- Ships ONNX model files alongside the binary
- CPU inference only (no GPU required)
- ~5-10ms per log line with 22-33M param models

**Distribution:**
- Go module (importable as a library)
- Standalone binary (runnable as a service)
- Docker image

**Dependencies:** Minimal. Standard library where possible. No heavy frameworks.

---

## What's Explicitly Out of Scope

- **LLM/agent integration** — Lumber is a log pipeline, not an agent framework
- **Database connectors** — separate concern
- **Codebase connectors** — separate concern
- **Generative models** — no text generation in the pipeline; embedding models only
- **UI/dashboard** — consumers build their own
- **Log storage/persistence** — Lumber processes and serves, it does not store
- **Policy/rule engine** — may be a future layer, not part of initial build

---

## Open Questions

1. **Taxonomy validation** — The initial taxonomy above is a draft. Needs validation against real log data from diverse applications to ensure coverage and granularity are right.
2. **Confidence thresholds** — Below what similarity score should a log be flagged as "unclassified"? How does this interact with the adaptive growing mechanism?
3. **Field extraction** — Classification tells you *what kind* of event it is, but doesn't extract structured fields (service name, host, port, user ID) from unstructured text. Is basic regex/heuristic extraction sufficient for v1, or is this a gap?
4. **Buffering and backpressure** — How much does the engine buffer when consumers are slow? Drop, queue, or push back on connectors?
5. **Multi-tenancy** — Does a single Lumber instance handle logs from multiple applications/environments, or is it one instance per source?
