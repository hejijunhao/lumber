# Lumber — Overview Blueprint

**Purpose:** Reference document for designing Lumber's overview webpage. Covers what Lumber is, what it does, why it matters, and how it works — both conceptually and technically.

---

## What Lumber Is

Lumber is a high-performance, open-source log normalization pipeline written in Go.

Raw logs go in — from any provider (Vercel, Fly.io, Supabase), any application, any format. Structured, classified, token-efficient canonical events come out. What consumes the output — LLM agents, dashboards, alerting systems, humans — is not Lumber's concern.

Lumber runs entirely on-device. No cloud API calls in the processing path. No GPU required. A single binary with two external dependencies.

---

## The Problem Lumber Solves

Logs are broken across two dimensions.

### Provider fragmentation

Every log provider exposes a different API, auth mechanism, and response format for the same fundamental thing — timestamped events from running software:

- Vercel: JSON arrays via REST with project-scoped tokens
- Fly.io: HTTP API with nested attribute structures
- Supabase: SQL-over-REST across 7 log tables
- AWS CloudWatch: paginated log groups/streams with IAM auth
- Datadog: proprietary query language with cursor-based pagination

Same data. Completely different shapes.

### Application-level chaos

Even within a single provider, every application logs differently. Four services reporting the same database connection failure produce four structurally unrelated log lines:

```
App A:  {"level":"error","msg":"connection timeout","service":"payments","ts":1708300800}
App B:  ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)
App C:  2026-02-19T12:00:00Z | ERR | request failed | {"status":503,"path":"/api/checkout"}
App D:  payments.timeout db-primary:5432 30s
```

No shared structure. No shared field names. No shared severity format. No shared timestamp format.

### Why this matters now

Every AI agent startup is connecting LLMs to logs. They're dumping raw log text into context windows. This is:

- **Token-wasteful** — raw logs are verbose, redundant, full of metadata the LLM doesn't need
- **Unreliable** — inconsistent formats mean the LLM reasons differently about equivalent events
- **Slow** — large payloads, no pre-processing, repeated parsing
- **Fragile** — each new log source requires custom integration code

Lumber eliminates all four problems. One pipeline normalizes every log source into a single schema, compacted for token efficiency, classified semantically — not by fragile regex, but by meaning.

---

## How It Works — Conceptual

```
Raw logs (any provider, any format)
   ↓  Connectors
Embed → Classify → Compact → Dedup
   ↓  Engine
Structured canonical events (JSON)
   ↓  Output
stdout · file · webhook (fan-out to multiple destinations)
```

Every log follows the same path:

1. A **connector** fetches the raw log from a provider's API, handling auth, pagination, and rate limiting
2. The **embedder** converts the log text into a 1024-dimensional vector using a local neural network
3. The **classifier** compares that vector against 42 pre-embedded taxonomy labels via cosine similarity — the best match becomes the log's category
4. The **compactor** strips noise, truncates stack traces, deduplicates repeated events, and optimizes for token efficiency
5. The **output layer** routes the classified event to one or more destinations — stdout, file, webhook — simultaneously

A raw log like this:

```
ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)
```

Becomes:

```json
{
  "type": "ERROR",
  "category": "connection_failure",
  "severity": "error",
  "timestamp": "2026-02-19T12:00:00Z",
  "summary": "UserService — connection refused (host=db-primary)",
  "confidence": 0.91
}
```

Regardless of whether that log came from Vercel, Fly.io, or Supabase. Regardless of how the application formatted it. Same input meaning → same output structure.

---

## Key USPs

### 1. Semantic classification, not regex

Traditional log parsers use pattern matching — regex, grok patterns, string matching. These break when log formats change, when new applications are onboarded, when developers write logs differently.

Lumber classifies by meaning. The embedding model converts log text into a vector that captures semantic content. A "connection refused" log and a "ECONNREFUSED dial tcp" log produce similar vectors and land in the same category — even though they share no words.

This is the same approach as CLIP for images (embed both sides into a shared vector space, compare with cosine similarity), applied to text-to-text log classification.

### 2. Fully local — zero cloud dependency

The embedding model (23M parameters, ~23MB) runs on-device via ONNX Runtime. No API calls. No cloud dependency. No usage fees. No data leaving the machine.

Lumber works fully offline after the one-time model download. This matters for:
- **Privacy** — log data never leaves the machine
- **Latency** — no network round-trips in the processing path
- **Cost** — no per-token or per-request charges
- **Reliability** — no external service outages

### 3. Token-aware output

Lumber's output is designed for LLM consumption:
- **Compact JSON schema** — 7 fields, no nesting, no redundancy
- **Configurable verbosity** — minimal (200 chars), standard (2000 chars), or full
- **Intelligent stack trace truncation** — preserves entry point and crash site, removes middle frames
- **Deduplication** — 47 identical errors become one event with `"count": 47`
- **High-cardinality field stripping** — trace IDs, span IDs, request IDs removed automatically

A log storm that would consume 50k tokens as raw text might produce 500 tokens of canonical events.

### 4. Opinionated taxonomy

Lumber ships with a curated taxonomy of 42 leaf labels across 8 root categories. Every log is classified into exactly one leaf. This is opinionated by design — a finite, known label set makes downstream consumption predictable and machine-friendly.

The taxonomy covers the full spectrum of what running software produces:

| Category | What it covers | Leaf count |
|----------|---------------|------------|
| **ERROR** | Connection failures, auth failures, timeouts, exceptions, OOM, rate limits | 9 |
| **REQUEST** | HTTP success/error/redirect, slow requests | 5 |
| **DEPLOY** | Build and deploy lifecycle events, rollbacks | 7 |
| **SYSTEM** | Health checks, scaling, resource alerts, process lifecycle | 5 |
| **ACCESS** | Login events, session management, permission changes, API keys | 5 |
| **PERFORMANCE** | Latency spikes, throughput drops, queue backlogs, cache events, slow queries | 5 |
| **DATA** | Database queries, migrations, replication | 3 |
| **SCHEDULED** | Cron job lifecycle | 3 |

### 5. Multiple output destinations

Lumber fans out classified events to multiple destinations simultaneously:

- **stdout** — NDJSON for piping to other tools, compact or pretty-printed
- **File** — NDJSON to disk with buffered I/O and size-based rotation
- **Webhook** — batched HTTP POST to any endpoint (Slack, PagerDuty, custom alerting)

Each destination runs independently. A slow webhook doesn't stall stdout. A file rotation doesn't block the pipeline. Async wrappers with configurable backpressure handle slow consumers.

### 6. Importable as a Go library

Beyond the CLI, Lumber exposes a public Go API. Any Go application can import the classification engine directly — no subprocess, no HTTP, no serialization overhead:

```go
l, _ := lumber.New(lumber.WithModelDir("models/"))
defer l.Close()

event, _ := l.Classify("ERROR: connection refused to db-primary:5432")
// event.Type == "ERROR"
// event.Category == "connection_failure"
// event.Confidence == 0.91
```

Batch classification is supported for throughput:

```go
events, _ := l.ClassifyBatch([]string{
    "ERROR: connection refused",
    "GET /api/users 200 OK 12ms",
    "Build succeeded in 34s",
})
```

The library API is the stable public contract. Internal packages can evolve freely without breaking consumers.

---

## How It Works — Technical

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        CONNECTORS                            │
│                                                              │
│    Vercel  │  Fly.io  │  Supabase  │  (extensible)          │
│                                                              │
│    Auth, pagination, rate limiting, polling                   │
│    One thin adapter per provider                              │
└──────────────────────────┬──────────────────────────────────┘
                           │  []RawLog
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                   CLASSIFICATION ENGINE                       │
│                                                              │
│    1. Embed     — log text → 1024-dim vector (ONNX)          │
│    2. Classify  — cosine similarity against 42 taxonomy labels│
│    3. Compact   — strip noise, truncate, summarize            │
│    4. Dedup     — collapse repeated events within window      │
└──────────────────────────┬──────────────────────────────────┘
                           │  []CanonicalEvent
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                          OUTPUT                              │
│                                                              │
│    Multi-router → stdout │ file │ webhook (async fan-out)    │
└─────────────────────────────────────────────────────────────┘
```

### The embedding pipeline

Every log passes through a four-stage embedding pipeline to become a vector:

```
text → tokenize (WordPiece) → ONNX inference → mean pool → dense projection → float32[1024]
```

| Stage | What it does |
|-------|-------------|
| **Tokenizer** | Pure-Go WordPiece subword tokenization against a 30,522-token vocabulary. Dynamic padding to actual sequence length (no wasted computation). |
| **ONNX session** | Runs the LEAF model (23M params, int8 quantized) via `onnxruntime-go`. Produces per-token hidden states. |
| **Mean pooling** | Averages token embeddings with attention mask weighting, so padding tokens don't contribute. |
| **Dense projection** | Multiplies pooled output by a learned weight matrix (loaded from safetensors) to produce the final 1024-dimensional vector. |

Single texts use `Embed()`. Multiple texts use `EmbedBatch()`, which packs inputs into a single ONNX call for throughput.

### The embedding model

Lumber uses MongoDB's LEAF (`mdbr-leaf-ir`) — ranked #1 on MTEB BEIR for models under 100M parameters.

| Property | Value |
|----------|-------|
| Parameters | 23M |
| On-disk size | ~23MB (int8 quantized) |
| Output dimension | 1024 (384-dim transformer + learned projection) |
| Tokenizer | WordPiece, 30,522 tokens, uncased |
| Max sequence length | 128 tokens |
| Inference | CPU-only, ~5-10ms per log line |
| Runtime | ONNX Runtime (Go bindings) |
| License | Apache 2.0 |

### Classification

At startup, all 42 taxonomy leaf labels are embedded using tuned semantic descriptions. These 42 vectors are computed once and held in memory.

At runtime, each log's embedding is compared against all 42 label vectors via cosine similarity. Best match wins. If the best score falls below the confidence threshold (default 0.5), the event is marked `UNCLASSIFIED`.

The 42-label taxonomy uses rich descriptions that capture synonyms and patterns, not just category names. For example, the `connection_failure` label's description includes "TCP connection refused, ECONNREFUSED, dial tcp, NXDOMAIN, connection reset, socket hangup" — this is what gets embedded, giving the classifier semantic reach beyond exact keyword matching.

### Compaction

Three verbosity tiers reduce token footprint:

| Verbosity | Text limit | Stack traces | JSON fields |
|-----------|-----------|--------------|-------------|
| **Minimal** | 200 runes | 5 frames + last 2 | Strip trace_id, span_id, request_id, etc. |
| **Standard** | 2000 runes | 10 frames + last 2 | Strip same high-cardinality fields |
| **Full** | No limit | Preserve all | Preserve all |

Stack trace detection covers Java (`at ...`), Go (`goroutine`, `.go:\d+`), and Python (`File "...", line N`) patterns. Middle frames are replaced with `"(N frames omitted)"` to preserve the entry point and crash site — the two most diagnostically valuable locations.

Summary extraction takes the first line of log text, truncated to 120 runes at a word boundary.

### Deduplication

Within a configurable time window (default 5s), events with the same `Type.Category` key are collapsed. Fifty identical `ERROR.connection_failure` events become one event with `"count": 50`. First-occurrence order is preserved.

The dedup buffer is bounded (default 1000 events). When full, it force-flushes — no events dropped, no unbounded memory growth during log storms.

### Connectors

Three connectors implement a common interface:

```go
type Connector interface {
    Stream(ctx context.Context, cfg ConnectorConfig) (<-chan RawLog, error)
    Query(ctx context.Context, cfg ConnectorConfig, params QueryParams) ([]RawLog, error)
}
```

- **Stream** — continuous polling for live log tails
- **Query** — bounded historical fetch

| Provider | API style | Pagination | Time filtering |
|----------|-----------|------------|----------------|
| Vercel | REST JSON | Cursor-based | Server-side (unix ms) |
| Fly.io | REST JSON | Token-based | Client-side half-open `[start, end)` |
| Supabase | SQL over REST | 24h window chunking | SQL WHERE clause |

All three share an HTTP client package with Bearer auth, retry with exponential backoff on 5xx, and `Retry-After` handling on 429.

New connectors register via `init()` into a global registry. The pipeline resolves them by name at startup. Adding a provider means implementing the interface — the pipeline, engine, and output layers don't change.

### Output layer

The output layer routes classified events to multiple destinations through a multi-router:

- **Multi-router** — fans out each event to all configured outputs. One output's failure doesn't prevent delivery to others.
- **Async wrapper** — decouples event production from consumption via a buffered channel. Slow outputs (webhook, file rotation) don't stall the pipeline. Configurable backpressure or drop-on-full semantics.
- **stdout backend** — NDJSON, compact or pretty-printed. Synchronous (fast, never fails meaningfully).
- **File backend** — NDJSON to disk with buffered I/O (64KB buffer, batches syscalls) and size-based rotation.
- **Webhook backend** — batched HTTP POST with configurable batch size, flush interval, and retry logic (exponential backoff on 5xx, no retry on 4xx).

### Public library API

The `pkg/lumber` package exposes a stable, minimal surface:

```go
// Create a Lumber instance (loads model, pre-embeds taxonomy)
l, err := lumber.New(
    lumber.WithModelDir("models/"),
    lumber.WithConfidenceThreshold(0.5),
    lumber.WithVerbosity("standard"),
)
defer l.Close()

// Classify single log
event, err := l.Classify("GET /api/users 200 OK 12ms")

// Classify batch (single ONNX inference call)
events, err := l.ClassifyBatch(texts)

// Classify with metadata
event, err := l.ClassifyLog(lumber.Log{
    Text:      "connection refused",
    Timestamp: time.Now(),
    Source:    "vercel",
})

// Inspect taxonomy
categories := l.Taxonomy()
```

The public types (`Event`, `Log`, `Category`, `Label`) are deliberately separate from internal types. Internal packages can be refactored freely without breaking consumers.

---

## Operating Modes

### CLI — Stream mode

Continuous log processing. Connectors poll for new logs, the engine classifies in real-time, output is streamed.

```bash
LUMBER_CONNECTOR=vercel LUMBER_API_KEY=tok_xxx lumber
```

### CLI — Query mode

One-shot historical fetch. Fetch, classify, output, exit.

```bash
lumber -mode query -from 2026-02-28T00:00:00Z -to 2026-02-28T01:00:00Z
```

### Library mode

Embedded in a Go application. No CLI, no connectors, no output layer — just the classification engine.

```go
l, _ := lumber.New(lumber.WithModelDir("models/"))
event, _ := l.Classify(logLine)
```

All modes produce identical canonical output — same schema, same classification, same taxonomy.

---

## Canonical Event Schema

Every log, regardless of source, is classified into this structure:

```json
{
  "type": "ERROR",
  "category": "connection_failure",
  "severity": "error",
  "timestamp": "2026-02-19T12:00:00Z",
  "summary": "UserService — connection refused (host=db-primary)",
  "confidence": 0.91,
  "raw": "ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)",
  "count": 1
}
```

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Root category: ERROR, REQUEST, DEPLOY, SYSTEM, ACCESS, PERFORMANCE, DATA, SCHEDULED |
| `category` | string | Leaf label within the root (e.g., `connection_failure`, `success`, `build_failed`) |
| `severity` | string | Normalized severity: `error`, `warning`, `info`, `debug` |
| `timestamp` | ISO 8601 | When the log was produced |
| `summary` | string | First line of log text, ≤120 runes, word-boundary truncated |
| `confidence` | float | Cosine similarity score (0–1). Omitted at minimal verbosity. |
| `raw` | string | Compacted original log text. Omitted at minimal verbosity. |
| `count` | int | Number of deduplicated events collapsed into this one. Omitted when 1. |

---

## Technical Profile

| Attribute | Value |
|-----------|-------|
| Language | Go 1.24 |
| External dependencies | 2 (`onnxruntime-go`, `golang.org/x/text`) |
| Embedding model | MongoDB LEAF, 23M params, ~23MB on disk |
| Embedding dimension | 1024 |
| Inference | CPU-only, ~5–10ms per log line |
| Taxonomy | 42 leaf labels, 8 root categories |
| Corpus accuracy | 100% top-1 on 104-entry labeled corpus |
| Output format | NDJSON (newline-delimited JSON) |
| License | Apache 2.0 |

---

## What Lumber Is Not

- **Not an agent framework** — Lumber is a log pipeline. It classifies logs; it doesn't react to them.
- **Not a log storage system** — Lumber processes and serves. It does not persist events.
- **Not a dashboard** — Consumers build their own UI on top of canonical events.
- **Not an LLM wrapper** — No generative models. No text generation. Embedding models only.
- **Not cloud-dependent** — No external API calls in the processing path. Fully offline after setup.

---

## Who It's For

### AI agent builders
LLM agents that monitor production systems need structured, token-efficient log data. Lumber turns raw log dumps into compact canonical events that an agent can reason about reliably — same schema every time, regardless of source.

### DevOps / platform teams
Teams running services across multiple providers (Vercel for frontend, Supabase for backend, Fly.io for edge) get a single normalized view of all their logs. One schema, one taxonomy, one tool.

### Observability pipelines
Lumber slots into existing log pipelines as a classification stage. Ingest from any source, classify, fan out to file/webhook/stdout for downstream consumers. The library API lets Go applications embed classification directly.

---

## Summary for Page Design

**Headline:** High-performance log normalization for AI agents and observability.

**Core message:** Raw logs from any provider, any format → structured, classified, token-efficient events. Runs locally. No cloud API calls. Single binary.

**Key differentiators to highlight:**
1. Semantic classification (embeddings, not regex)
2. Fully local / offline (privacy, speed, cost)
3. Token-aware output (built for LLM consumption)
4. Opinionated 42-label taxonomy (predictable downstream consumption)
5. Multi-destination output (stdout, file, webhook — simultaneously)
6. Importable Go library (embed the engine in your own app)
7. Minimal footprint (2 dependencies, 23MB model, CPU-only)
