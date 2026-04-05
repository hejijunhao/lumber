<p align="center">
  <img src="logo.svg" alt="Lumber" width="120" />
</p>

<h1 align="center">Lumber</h1>

<p align="center">
  <strong>High-performance log normalization pipeline written in Go.</strong><br/>
  Raw logs go in — from any provider, any format.<br/>
  Structured, canonical, token-efficient events come out.
</p>

<p align="center">
  <a href="#quickstart">Quickstart</a> ·
  <a href="#how-it-works">How It Works</a> ·
  <a href="#library-usage">Library API</a> ·
  <a href="#connectors">Connectors</a> ·
  <a href="#taxonomy">Taxonomy</a> ·
  <a href="docs/integration-guide.md">Integration Guide</a> ·
  <a href="docs/changelog.md">Changelog</a>
</p>

<p align="center">
  <code>v0.10.6</code> · Apache 2.0 · Go 1.24+
</p>

---

## Why

Every log provider has a different API, auth mechanism, and response format. Every application logs differently. Lumber normalizes all of it into a single schema using a local embedding model and semantic classification — no cloud API calls, no LLM dependency.

This matters most for AI agent workflows that consume logs. Raw log dumps waste tokens, break on inconsistent formats, and require per-source integration code. Lumber solves that.

---

## How It Works

```
Raw logs (Vercel, Fly.io, Supabase, …)
   ↓  connectors
Embed → Classify → Canonicalize → Compact
   ↓  engine
Structured canonical events (JSON)
```

1. **Connectors** ingest raw logs from providers via a unified interface (stream or query)
2. **Embedder** converts each log line into a 1024-dim vector using a local ONNX model (~23MB, CPU-only)
3. **Classifier** compares the vector against 42 pre-embedded taxonomy labels via cosine similarity
4. **Compactor** strips noise, truncates stack traces, and deduplicates repeated events

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

---

## Install

### Pre-built binaries (recommended)

Download the latest release for your platform from
[GitHub Releases](https://github.com/kaminocorp/lumber/releases):

| Platform | Archive |
|----------|---------|
| Linux x86_64 | `lumber-vX.Y.Z-linux-amd64.tar.gz` |
| Linux ARM64 | `lumber-vX.Y.Z-linux-arm64.tar.gz` |
| macOS Apple Silicon | `lumber-vX.Y.Z-darwin-arm64.tar.gz` |
| macOS Intel | `lumber-vX.Y.Z-darwin-amd64.tar.gz` |

Extract and run:

```bash
tar xzf lumber-vX.Y.Z-linux-amd64.tar.gz
cd lumber-vX.Y.Z-linux-amd64
bin/lumber -version
```

The release tarball is self-contained — binary, model files, and ONNX
Runtime library are all included. No additional downloads required.

### Build from source

Requires Go 1.24+ and curl.

```bash
git clone https://github.com/kaminocorp/lumber.git
cd lumber
make download-model   # fetches model files + ONNX Runtime for your platform
make build
bin/lumber -version
```

### Go library

```bash
go get github.com/kaminocorp/lumber
```

See [Library Usage](#library-usage) below.

---

## Quickstart

### Interactive setup

Run `lumber` with no arguments to launch the setup wizard. It walks through connector selection, credentials, output destinations, and auto-downloads model files if needed.

```bash
./bin/lumber
```

### Pipe logs from stdin

```bash
cat /var/log/app.log | ./bin/lumber
# or
tail -f /var/log/app.log | ./bin/lumber
```

Lumber auto-detects piped input and classifies each line.

### Stream from a provider

```bash
export LUMBER_CONNECTOR=vercel
export LUMBER_API_KEY=your-token-here
export LUMBER_VERCEL_PROJECT_ID=prj_your-project-id

./bin/lumber
```

### Classify a local file

```bash
./bin/lumber -connector file -file /var/log/app.log
```

### Query historical logs

```bash
./bin/lumber -mode query \
  -from 2026-02-24T00:00:00Z \
  -to 2026-02-24T01:00:00Z
```

### Check version

```bash
./bin/lumber -version
```

---

## CLI Flags

Flags override environment variables when set explicitly.

```
lumber [flags]

  -mode string        Pipeline mode: stream or query
  -connector string   Connector: vercel, flyio, supabase
  -from string        Query start time (RFC3339)
  -to string          Query end time (RFC3339)
  -limit int          Query result limit
  -verbosity string   Verbosity: minimal, standard, full
  -pretty             Pretty-print JSON output
  -log-level string   Log level: debug, info, warn, error
  -version            Print version and exit
```

Examples:

```bash
# Stream from Fly.io with debug logging
./bin/lumber -connector flyio -log-level debug

# Query last hour, pretty-printed
./bin/lumber -mode query -from 2026-02-24T07:00:00Z -to 2026-02-24T08:00:00Z -pretty

# Minimal verbosity for token-efficient output
./bin/lumber -verbosity minimal
```

---

## Connectors

Five connectors are implemented. Each produces `RawLog` entries that feed into the classification engine.

### Cloud providers

| Connector | Env Var | Description |
|-----------|---------|-------------|
| **Vercel** | `LUMBER_CONNECTOR=vercel` | REST API, project-scoped tokens |
| **Fly.io** | `LUMBER_CONNECTOR=flyio` | HTTP logs API |
| **Supabase** | `LUMBER_CONNECTOR=supabase` | Analytics API, multi-table queries |

### Local sources

| Connector | Env Var | Description |
|-----------|---------|-------------|
| **stdin** | Auto-detected | Pipe logs directly: `cat app.log \| lumber` |
| **file** | `LUMBER_CONNECTOR=file` | Read from a local log file |

### Provider configuration

```bash
# Vercel
export LUMBER_CONNECTOR=vercel
export LUMBER_API_KEY=your-vercel-token
export LUMBER_VERCEL_PROJECT_ID=prj_xxx
export LUMBER_VERCEL_TEAM_ID=team_xxx   # optional

# Fly.io
export LUMBER_CONNECTOR=flyio
export LUMBER_API_KEY=your-fly-token
export LUMBER_FLY_APP_NAME=your-app-name

# Supabase
export LUMBER_CONNECTOR=supabase
export LUMBER_API_KEY=your-supabase-service-key
export LUMBER_SUPABASE_PROJECT_REF=your-project-ref
export LUMBER_SUPABASE_TABLES=edge_logs,postgres_logs  # optional, defaults to all
```

---

## Configuration

### Core settings

| Variable | Default | Description |
|---|---|---|
| `LUMBER_CONNECTOR` | `vercel` | Log provider: `vercel`, `flyio`, `supabase` |
| `LUMBER_API_KEY` | — | Provider API key/token |
| `LUMBER_ENDPOINT` | — | Provider API endpoint URL override |
| `LUMBER_MODE` | `stream` | Pipeline mode: `stream` or `query` |
| `LUMBER_VERBOSITY` | `standard` | Output verbosity: `minimal`, `standard`, `full` |
| `LUMBER_OUTPUT` | `stdout` | Output destination |
| `LUMBER_OUTPUT_PRETTY` | `false` | Pretty-print JSON output |

### Engine settings

| Variable | Default | Description |
|---|---|---|
| `LUMBER_MODEL_PATH` | `models/model_quantized.onnx` | Path to ONNX model file |
| `LUMBER_VOCAB_PATH` | `models/vocab.txt` | Path to tokenizer vocabulary |
| `LUMBER_PROJECTION_PATH` | `models/2_Dense/model.safetensors` | Path to projection weights |
| `LUMBER_CONFIDENCE_THRESHOLD` | `0.5` | Min confidence to classify (0–1) |
| `LUMBER_DEDUP_WINDOW` | `5s` | Dedup window duration (`0` disables) |
| `LUMBER_MAX_BUFFER_SIZE` | `1000` | Max events buffered before force flush |

### Operational settings

| Variable | Default | Description |
|---|---|---|
| `LUMBER_LOG_LEVEL` | `info` | Internal log level: `debug`, `info`, `warn`, `error` |
| `LUMBER_SHUTDOWN_TIMEOUT` | `10s` | Max drain time on shutdown |
| `LUMBER_POLL_INTERVAL` | provider default | Polling interval for stream mode |

### Provider-specific settings

| Variable | Provider | Description |
|---|---|---|
| `LUMBER_VERCEL_PROJECT_ID` | Vercel | Vercel project ID |
| `LUMBER_VERCEL_TEAM_ID` | Vercel | Vercel team ID (optional) |
| `LUMBER_FLY_APP_NAME` | Fly.io | Fly.io application name |
| `LUMBER_SUPABASE_PROJECT_REF` | Supabase | Supabase project reference |
| `LUMBER_SUPABASE_TABLES` | Supabase | Comma-separated log table list |

### Verbosity levels

| Level | Behavior |
|---|---|
| `minimal` | Raw logs truncated to 200 characters |
| `standard` | Raw logs truncated to 2000 characters |
| `full` | Complete raw logs preserved |

---

## Taxonomy

Lumber ships with 42 leaf labels organized under 8 top-level categories. Every log is classified into exactly one leaf. The taxonomy is opinionated by design — a finite label set makes downstream consumption predictable.

| Category | Labels |
|---|---|
| **ERROR** | connection_failure, auth_failure, authorization_failure, timeout, runtime_exception, validation_error, out_of_memory, rate_limited, dependency_error |
| **REQUEST** | success, client_error, server_error, redirect, slow_request |
| **DEPLOY** | build_started, build_succeeded, build_failed, deploy_started, deploy_succeeded, deploy_failed, rollback |
| **SYSTEM** | health_check, scaling_event, resource_alert, process_lifecycle, config_change |
| **ACCESS** | login_success, login_failure, session_expired, permission_change, api_key_event |
| **PERFORMANCE** | latency_spike, throughput_drop, queue_backlog, cache_event, db_slow_query |
| **DATA** | query_executed, migration, replication |
| **SCHEDULED** | cron_started, cron_completed, cron_failed |

Classification uses cosine similarity between the log's embedding vector and pre-embedded taxonomy label descriptions. Labels below the confidence threshold (default 0.5) are marked `UNCLASSIFIED`.

---

## Embedding Model

Lumber uses [MongoDB LEAF (mdbr-leaf-mt)](https://huggingface.co/MongoDB/mdbr-leaf-mt), a 23M parameter text embedding model. Runs locally via ONNX Runtime — no external API calls, no GPU required.

| Property | Value |
|---|---|
| Size | ~23MB (int8 quantized) |
| Output dimension | 1024 (384-dim transformer + learned projection) |
| Tokenizer | WordPiece (30,522 tokens, lowercase) |
| Max sequence length | 128 tokens |
| Runtime | ONNX Runtime via [onnxruntime-go](https://github.com/yalue/onnxruntime_go) |

---

## Library Usage

Lumber can be imported as a Go library. Classify log text directly in your application — no subprocess, no stdout parsing, no network calls at runtime.

```bash
go get github.com/kaminocorp/lumber
```

```go
import "github.com/kaminocorp/lumber/pkg/lumber"
```

### Auto-download (recommended for getting started)

```go
// Downloads ~35-60MB of model files on first call.
// Cached at ~/.cache/lumber — subsequent calls are instant.
l, err := lumber.New(lumber.WithAutoDownload())
if err != nil {
    log.Fatal(err)
}
defer l.Close()

event, _ := l.Classify("ERROR: connection refused to db-primary:5432")
fmt.Println(event.Type, event.Category) // ERROR connection_failure
```

### Pre-downloaded models (recommended for production/Docker)

```go
// Use make download-model or Dockerfile COPY stage to prepare the directory.
l, err := lumber.New(lumber.WithModelDir("/opt/lumber/models"))
if err != nil {
    log.Fatal(err)
}
defer l.Close()
```

### Batch classification

```go
// Single batched ONNX inference call — ~10x faster per line than looping Classify
events, _ := l.ClassifyBatch([]string{
    "ERROR: connection refused",
    "GET /api/users 200 OK 12ms",
    "Build succeeded in 45s",
})
for _, e := range events {
    fmt.Printf("%-12s %-20s (%.2f)\n", e.Type, e.Category, e.Confidence)
}
```

### Structured input with metadata

```go
event, _ := l.ClassifyLog(lumber.Log{
    Text:      "ERROR: connection refused",
    Timestamp: time.Now(),
    Source:    "vercel",
    Metadata:  map[string]any{"project": "api-prod"},
})
```

### Taxonomy introspection

```go
for _, cat := range l.Taxonomy() {
    fmt.Printf("%s: %d labels\n", cat.Name, len(cat.Labels))
}
```

### API summary

| Method | Description | Latency |
|--------|-------------|---------|
| `New(opts...)` | Initialize engine, load model, pre-embed taxonomy | ~100–300ms (once) |
| `Classify(text)` | Classify a single log line | ~5–10ms |
| `ClassifyBatch(texts)` | Batch classify (single ONNX call) | ~50–80ms / 100 lines |
| `ClassifyLog(log)` | Classify with timestamp, source, metadata | ~5–10ms |
| `ClassifyLogs(logs)` | Batch classify structured logs | ~50–80ms / 100 logs |
| `Taxonomy()` | Return the full taxonomy tree (read-only) | ~0ms |
| `Close()` | Release ONNX runtime resources | — |

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithAutoDownload()` | disabled | Auto-fetch model + ORT on first use, cache locally |
| `WithModelDir(dir)` | `"models"` | Directory containing model files |
| `WithModelPaths(m, v, p)` | — | Explicit paths for model, vocab, projection |
| `WithCacheDir(dir)` | `~/.cache/lumber` | Override auto-download cache location |
| `WithConfidenceThreshold(t)` | `0.5` | Min cosine similarity for classification (0–1) |
| `WithVerbosity(v)` | `"standard"` | Summary compaction: `minimal`, `standard`, `full` |

The `Lumber` instance is safe for concurrent use from multiple goroutines. Create once at startup, reuse across requests, close on shutdown.

For integration patterns (monitoring agents, HTTP middleware, batch workers), performance tuning, and troubleshooting, see the **[Integration Guide](docs/integration-guide.md)**.

---

## Output Destinations

Lumber supports multiple simultaneous output destinations.

| Destination | Env Var | CLI Flag | Behavior |
|---|---|---|---|
| **stdout** | (always on) | — | NDJSON to stdout (synchronous) |
| **File** | `LUMBER_OUTPUT_FILE` | `-output-file` | NDJSON to file with optional rotation |
| **Webhook** | `LUMBER_WEBHOOK_URL` | `-webhook-url` | Batched HTTP POST with retry |

File and webhook outputs run asynchronously — they don't stall the pipeline. Webhook uses drop-on-full semantics (lossy by design for non-critical destinations).

```bash
# Stream to stdout + file + webhook simultaneously
export LUMBER_OUTPUT_FILE=/var/log/lumber/events.jsonl
export LUMBER_OUTPUT_FILE_MAX_SIZE=104857600  # 100MB rotation
export LUMBER_WEBHOOK_URL=https://hooks.example.com/lumber
./bin/lumber
```

---

## Project Structure

```
cmd/lumber/              CLI entrypoint + interactive setup wizard
pkg/lumber/              Public library API (Classify, ClassifyBatch, Taxonomy)
internal/
  cli/                   Interactive setup wizard (charmbracelet/huh)
  config/                Environment + CLI flag configuration, validation
  connector/             Connector interface, registry
    vercel/              Vercel REST API connector
    flyio/               Fly.io HTTP logs connector
    supabase/            Supabase Analytics connector
    stdin/               Stdin connector (piped input)
    file/                Local file connector
    httpclient/          Shared HTTP client (auth, retry, rate limits)
  download/              Model + ORT auto-download, platform detection
  engine/                Classification engine orchestration
    embedder/            ONNX Runtime embedding (tokenizer, projection)
    classifier/          Cosine similarity classification
    compactor/           Token-aware log compaction
    dedup/               Event deduplication
    taxonomy/            Taxonomy tree and default labels
    testdata/            153-entry labeled test corpus
  logging/               Structured internal logging (slog)
  model/                 Domain types (RawLog, CanonicalEvent, TaxonomyNode)
  output/                Output formatting and writers
    stdout/              NDJSON stdout writer
    file/                NDJSON file writer with rotation
    webhook/             Batched HTTP POST with retry
    multi/               Fan-out to multiple outputs
    async/               Channel-based async wrapper
  pipeline/              Stream and Query orchestration, buffering
models/                  ONNX model files (downloaded via make)
docs/                    Architecture, plans, completion notes, changelog
```

---

## Development

```bash
make build           # Build binary to bin/lumber
make test            # Run all tests
make lint            # Run golangci-lint
make clean           # Remove build artifacts
make download-model  # Fetch ONNX model + tokenizer from HuggingFace
```

---

## Status

Lumber is production-ready for its core use case: classifying logs from supported providers and local sources into a structured canonical schema.

### Completed

- [x] ONNX Runtime integration and local embedding (~23MB model, CPU-only)
- [x] Pure-Go WordPiece tokenizer, mean pooling, dense projection (1024-dim)
- [x] 42-leaf taxonomy with semantic pre-embedding — 100% accuracy on 153-entry corpus
- [x] Log connectors: Vercel, Fly.io, Supabase, stdin, local file
- [x] Shared HTTP client with retry, rate limiting, response size limits
- [x] Pipeline integration (stream + query modes) with buffering and dedup
- [x] Multi-output architecture (stdout + file rotation + webhook with retry)
- [x] Public library API (`pkg/lumber`) — safe for concurrent use
- [x] Interactive setup wizard with model auto-download
- [x] Distribution: multi-platform binaries, GitHub Releases, Go module
- [x] Production hardening: thread safety, path injection protection, credential redaction, NaN/Inf guards, decompression bomb protection, graceful shutdown

### Roadmap

- [ ] Additional connectors (AWS CloudWatch, Datadog, Grafana Loki)
- [ ] HTTP server mode
- [ ] Adaptive taxonomy (self-growing/trimming)
- [ ] Field extraction (structured field parsing from unstructured text)
- [ ] Performance benchmarks

See [docs/changelog.md](docs/changelog.md) for detailed release notes and [docs/plans/post-beta-proposals.md](docs/plans/post-beta-proposals.md) for the full roadmap.

---

<p align="center">
  <a href="LICENSE">Apache 2.0</a>
</p>
