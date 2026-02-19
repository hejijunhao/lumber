# Lumber Beta Roadmap

## Overview

This roadmap takes Lumber from its current scaffolding (compiles, correct interfaces, stubs) to a functioning beta: a pipeline that ingests real logs from at least one provider, classifies them semantically using a local embedding model, and outputs structured canonical events.

**Starting point:** Scaffolding complete. All interfaces defined, project compiles, classifier and compactor have real logic, embedder and Vercel connector are stubbed.

**Beta definition:** Lumber can be pointed at a Vercel project, stream or query real logs, classify every log line against the taxonomy with meaningful accuracy, and output structured JSON events — all running locally with no external API dependencies in the processing path.

---

## Phase 1: Embedding Engine

**Goal:** A working local embedder that converts text → vectors using ONNX Runtime.

This is the critical-path blocker. Nothing downstream (taxonomy pre-embedding, real classification, end-to-end pipeline) works until this does.

**Scope:**
- Integrate `onnxruntime-go` as the first external dependency
- Select and download a model (likely all-MiniLM-L6-v2 or similar 22-33M param model)
- Implement tokenization (WordPiece or SentencePiece depending on model)
- Wire up `Embed()` and `EmbedBatch()` to produce real vectors
- Update `make download-model` to fetch the chosen model
- Benchmark: confirm <10ms per log line on CPU

**Depends on:** Nothing (scaffolding complete)
**Unblocks:** Phases 2, 3, 4

**Plan file:** `docs/plans/phase-1-embedding-engine.md`

---

## Phase 2: Classification Pipeline

**Goal:** End-to-end classify: raw text in → canonical event out, tested with synthetic logs.

With the embedder producing real vectors, this phase wires up taxonomy pre-embedding and validates that the full engine pipeline (embed → classify → canonicalize → compact) produces correct, meaningful results.

**Scope:**
- Implement taxonomy pre-embedding at startup (embed all 34+ leaf labels)
- Align the default taxonomy with the vision doc's full label set (~40-50 labels)
- Tune confidence threshold using synthetic log samples
- Improve severity inference beyond the current naive type-based mapping
- Build a synthetic test corpus: ~100 representative log lines spanning all taxonomy categories
- Unit tests for the full engine path
- Validate classification accuracy against the test corpus (target: >80% correct at top-1)

**Depends on:** Phase 1
**Unblocks:** Phase 4 (can run end-to-end with synthetic data)

**Plan file:** `docs/plans/phase-2-classification-pipeline.md`

---

## Phase 3: Vercel Connector

**Goal:** Ingest real logs from Vercel — both streaming and historical query.

This is the first real-world data source. It validates that the connector interface works with actual provider APIs and produces well-formed `RawLog` entries.

**Scope:**
- Implement Vercel REST API client (log drains endpoint)
- Handle authentication (project-scoped tokens)
- Implement `Query()` — historical log fetch with pagination
- Implement `Stream()` — polling-based live tail (Vercel doesn't offer true streaming; periodic poll with cursor)
- Parse Vercel's log format into `RawLog` (timestamp extraction, metadata mapping)
- Rate limiting and error handling
- Integration test with a real Vercel project (or recorded fixtures)

**Depends on:** Scaffolding (interfaces defined). Can be developed in parallel with Phases 1-2, but only tested end-to-end after Phase 2.
**Unblocks:** Phase 5 (real data through the full pipeline)

**Plan file:** `docs/plans/phase-3-vercel-connector.md`

---

## Phase 4: Compactor & Output Hardening

**Goal:** Compaction produces genuinely token-efficient output. Stdout output is production-quality.

The current compactor does basic truncation. This phase makes it smart — deduplication, structured field stripping, and verbosity-aware formatting that actually reduces token usage meaningfully.

**Scope:**
- Implement event deduplication with counted summaries (e.g., "connection_failure ×47 in last 5m")
- Smart truncation: preserve first/last lines of stack traces, strip middle frames
- Structured field stripping: remove fields that add tokens but not signal (trace IDs, request IDs at minimal verbosity)
- JSON output formatting options (compact vs. pretty)
- Measure token counts before/after compaction to validate efficiency gains
- Unit tests for all compaction modes

**Depends on:** Phase 2 (needs real canonical events to compact meaningfully)
**Unblocks:** Phase 6

**Plan file:** `docs/plans/phase-4-compactor-output.md`

---

## Phase 5: Pipeline Integration & Resilience

**Goal:** The full pipeline (connector → engine → output) runs reliably against real log data with proper error handling, buffering, and graceful shutdown.

This is where we go from "the pieces work" to "the system works."

**Scope:**
- Buffering and backpressure between pipeline stages (bounded channels)
- Error handling strategy: per-log errors vs. fatal errors (don't crash on one bad log)
- Graceful shutdown: drain in-flight logs on SIGTERM, bounded shutdown timeout
- Logging/observability for Lumber itself (structured internal logging, not to be confused with the logs it processes)
- CLI improvements: flags for mode (stream/query), time range, verbosity, connector selection
- Config validation at startup (fail fast on bad config)
- End-to-end integration test: Vercel connector → engine → stdout, processing real logs

**Depends on:** Phases 2, 3, 4
**Unblocks:** Phase 6

**Plan file:** `docs/plans/phase-5-pipeline-integration.md`

---

## Phase 6: Beta Validation & Polish

**Goal:** Lumber works reliably enough to hand to someone else. Documentation, error messages, and edge cases are handled.

**Scope:**
- Run against real Vercel log traffic for sustained periods (hours, not seconds)
- Identify and fix misclassifications — tune taxonomy labels/descriptions
- Handle edge cases: empty logs, binary content, extremely long lines, non-UTF8
- `make build` produces a self-contained binary
- README with quickstart (install, configure, run)
- CLI help text and error messages are clear
- Changelog updated through beta

**Depends on:** Phase 5
**Unblocks:** Beta release

**Plan file:** `docs/plans/phase-6-beta-validation.md`

---

## Dependency Graph

```
Phase 1: Embedding Engine
   │
   ├──→ Phase 2: Classification Pipeline
   │       │
   │       ├──→ Phase 4: Compactor & Output Hardening
   │       │       │
   │       │       └──→ Phase 5: Pipeline Integration ──→ Phase 6: Beta Validation
   │       │               ▲
   │       └───────────────┘
   │
   └──→ Phase 3: Vercel Connector (parallel with 1-2)
           │
           └──→ Phase 5: Pipeline Integration
```

Phase 3 (Vercel) can proceed in parallel with Phases 1-2 since the connector interface is already defined. It joins the critical path at Phase 5 when we integrate everything.

---

## What's explicitly deferred to post-beta

These are in the vision doc but not needed for a functioning beta:

- **Adaptive taxonomy** (self-growing/trimming) — requires significant log volume and observation time
- **Additional connectors** (AWS, Fly.io, Datadog, Grafana) — beta validates the pattern with Vercel; others follow the same interface
- **gRPC/WebSocket output** — stdout + JSON is sufficient for beta
- **Webhook push output** — post-beta
- **Field extraction** (regex/heuristic parsing of service names, hosts, etc.) — valuable but not core to classification
- **Docker image** — post-beta distribution concern
- **Multi-tenancy** — single-instance-per-source is fine for beta
- **Library API** (public `pkg/` surface) — everything stays in `internal/` for beta

---

## Notes

- Each phase gets its own detailed implementation plan in `docs/plans/` before work begins.
- Each completed phase gets a completion summary in `docs/completions/`.
- The phases are sized to be individually shippable — after each phase, something new works that didn't before.
- Phase numbering reflects the dependency order, not strict sequencing. Phases 1 and 3 can (and should) overlap.
