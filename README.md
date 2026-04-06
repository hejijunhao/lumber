<p align="center">
  <img src="logo.svg" alt="Lumber" width="120" />
</p>

<h1 align="center">Lumber</h1>

<p align="center">
  <strong>Turn messy logs into structured events. Locally. Instantly.</strong>
</p>

<p align="center">
  <code>v0.10.6</code>&ensp;|&ensp;Apache 2.0&ensp;|&ensp;Go 1.24+&ensp;|&ensp;No cloud dependencies
</p>

<p align="center">
  <a href="#-get-started">Get Started</a>&ensp;&bull;&ensp;
  <a href="#-how-it-works">How It Works</a>&ensp;&bull;&ensp;
  <a href="#-use-as-a-go-library">Library API</a>&ensp;&bull;&ensp;
  <a href="#-connectors">Connectors</a>&ensp;&bull;&ensp;
  <a href="docs/changelog.md">Changelog</a>
</p>

---

Lumber is a log normalization pipeline. It takes raw logs from **any source** — cloud providers, local files, stdin — and classifies each line into a **structured canonical event** using a local AI embedding model. No API keys needed. Runs entirely on your machine.

```
             your logs                            Lumber output
                                                  
 ERROR [2026-02-19] UserService        {
   — connection refused                   "type": "ERROR",
   (host=db-primary, port=5432)    →      "category": "connection_failure",
                                          "severity": "error",
                                          "summary": "UserService — connection refused",
                                          "confidence": 0.91
                                        }
```

---

## Why Lumber?

| Problem | How Lumber solves it |
|---|---|
| Every log provider has a different format | Connectors normalize ingestion; one schema out |
| Every app logs differently | Semantic classification — meaning, not pattern matching |
| Raw logs waste LLM tokens | Compact, structured output designed for agent consumption |
| Cloud classification APIs add latency and cost | 23MB local model, ~5ms per log, fully offline |

---

## Get Started

### Option A: Try it right now (pipe any log file)

```bash
# 1. Clone and build
git clone https://github.com/kaminocorp/lumber.git && cd lumber
make download-model && make build

# 2. Classify logs
cat /var/log/system.log | ./bin/lumber
```

That's it. Each line prints as a classified JSON event.

### Option B: Interactive setup wizard

```bash
./bin/lumber
```

Run with no arguments — the wizard walks you through source selection, credentials, output options, and downloads model files automatically.

### Option C: Download a pre-built release

Grab the latest binary for your platform from [GitHub Releases](https://github.com/kaminocorp/lumber/releases). The tarball is self-contained — binary, model, and runtime included.

```bash
tar xzf lumber-vX.Y.Z-linux-amd64.tar.gz
cd lumber-vX.Y.Z-linux-amd64
cat /var/log/app.log | bin/lumber
```

> **Supported platforms:** Linux x86_64, Linux ARM64, macOS Apple Silicon, macOS Intel

### Option D: Use as a Go library

```bash
go get github.com/kaminocorp/lumber
```

```go
l, err := lumber.New(lumber.WithAutoDownload()) // downloads ~50MB on first use, cached after
if err != nil {
    log.Fatal(err)
}
defer l.Close()

event, _ := l.Classify("ERROR: connection refused to db-primary:5432")
fmt.Println(event.Type, event.Category) // ERROR connection_failure
```

---

## Common Usage Patterns

### Stream from a cloud provider

```bash
export LUMBER_CONNECTOR=vercel
export LUMBER_API_KEY=your-token
export LUMBER_VERCEL_PROJECT_ID=prj_xxx

./bin/lumber
```

### Classify a local log file

```bash
./bin/lumber -connector file -file /var/log/app.log
```

### Query historical logs

```bash
./bin/lumber -mode query \
  -connector vercel \
  -from 2026-02-24T00:00:00Z \
  -to 2026-02-24T01:00:00Z
```

### Output to file + webhook simultaneously

```bash
export LUMBER_OUTPUT_FILE=/var/log/lumber/events.jsonl
export LUMBER_WEBHOOK_URL=https://hooks.example.com/lumber
./bin/lumber
```

---

## How It Works

```
Raw logs (Vercel, Fly.io, Supabase, stdin, file)
   |
   |  1. CONNECTORS — unified ingestion from any source
   v
   |  2. EMBED — log line -> 1024-dim vector (local ONNX model, ~5ms)
   v
   |  3. CLASSIFY — cosine similarity against 42 taxonomy labels
   v
   |  4. COMPACT — strip noise, truncate stack traces, deduplicate
   |
   v
Structured canonical events (NDJSON)
```

The embedding model ([MongoDB LEAF](https://huggingface.co/MongoDB/mdbr-leaf-mt), 23M params) runs locally via ONNX Runtime. No external calls. No GPU needed. Works on an 8GB MacBook Air.

### The Taxonomy

Every log is classified into one of **42 leaf labels** under 8 categories:

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

Logs below the confidence threshold (default 0.5) are marked `UNCLASSIFIED`.

---

## Use as a Go Library

Classify logs directly in your Go application — no subprocess, no network calls.

### Batch classification

```go
// Single ONNX inference call — ~10x faster than looping Classify
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

### API reference

| Method | Description | Latency |
|--------|-------------|---------|
| `New(opts...)` | Initialize engine, load model, pre-embed taxonomy | ~100-300ms (once) |
| `Classify(text)` | Classify a single log line | ~5-10ms |
| `ClassifyBatch(texts)` | Batch classify (single ONNX call) | ~50-80ms / 100 lines |
| `ClassifyLog(log)` | Classify with timestamp, source, metadata | ~5-10ms |
| `ClassifyLogs(logs)` | Batch classify structured logs | ~50-80ms / 100 logs |
| `Taxonomy()` | Return the full taxonomy tree | ~0ms |
| `Close()` | Release ONNX runtime resources | - |

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithAutoDownload()` | disabled | Auto-fetch model + ORT on first use, cache locally |
| `WithModelDir(dir)` | `"models"` | Directory containing model files |
| `WithModelPaths(m, v, p)` | - | Explicit paths for model, vocab, projection |
| `WithCacheDir(dir)` | `~/.cache/lumber` | Override auto-download cache location |
| `WithConfidenceThreshold(t)` | `0.5` | Min cosine similarity for classification (0-1) |
| `WithVerbosity(v)` | `"standard"` | Summary compaction: `minimal`, `standard`, `full` |

The `Lumber` instance is safe for concurrent use. Create once at startup, share across goroutines, close on shutdown.

For integration patterns (monitoring agents, HTTP middleware, batch workers) and performance tuning, see the **[Integration Guide](docs/integration-guide.md)**.

---

## Connectors

### Cloud providers

| Connector | Config | Required |
|-----------|--------|----------|
| **Vercel** | `LUMBER_CONNECTOR=vercel` | `LUMBER_API_KEY`, `LUMBER_VERCEL_PROJECT_ID` |
| **Fly.io** | `LUMBER_CONNECTOR=flyio` | `LUMBER_API_KEY`, `LUMBER_FLY_APP_NAME` |
| **Supabase** | `LUMBER_CONNECTOR=supabase` | `LUMBER_API_KEY`, `LUMBER_SUPABASE_PROJECT_REF` |

### Local sources

| Connector | Config | Notes |
|-----------|--------|-------|
| **stdin** | Auto-detected when input is piped | `cat app.log \| lumber` |
| **file** | `LUMBER_CONNECTOR=file`, `-file PATH` | Reads a local log file |

<details>
<summary><strong>Full provider configuration examples</strong></summary>

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
export LUMBER_SUPABASE_TABLES=edge_logs,postgres_logs  # optional
```

</details>

---

## CLI Reference

```
lumber [flags]

  -mode string        Pipeline mode: stream or query (default: stream)
  -connector string   Connector: vercel, flyio, supabase, file
  -file string        Log file path (for file connector)
  -from string        Query start time (RFC3339)
  -to string          Query end time (RFC3339)
  -limit int          Query result limit
  -verbosity string   Output: minimal, standard, full (default: standard)
  -pretty             Pretty-print JSON output
  -log-level string   Log level: debug, info, warn, error (default: info)
  -version            Print version and exit
```

---

## Configuration

All settings can be set via environment variables. CLI flags override env vars.

<details>
<summary><strong>Core settings</strong></summary>

| Variable | Default | Description |
|---|---|---|
| `LUMBER_CONNECTOR` | `vercel` | Log provider |
| `LUMBER_API_KEY` | - | Provider API key/token |
| `LUMBER_ENDPOINT` | - | Provider API endpoint override |
| `LUMBER_MODE` | `stream` | Pipeline mode: `stream` or `query` |
| `LUMBER_VERBOSITY` | `standard` | Output verbosity: `minimal`, `standard`, `full` |
| `LUMBER_OUTPUT_PRETTY` | `false` | Pretty-print JSON output |

</details>

<details>
<summary><strong>Engine settings</strong></summary>

| Variable | Default | Description |
|---|---|---|
| `LUMBER_MODEL_PATH` | `models/model_quantized.onnx` | Path to ONNX model file |
| `LUMBER_VOCAB_PATH` | `models/vocab.txt` | Path to tokenizer vocabulary |
| `LUMBER_PROJECTION_PATH` | `models/2_Dense/model.safetensors` | Path to projection weights |
| `LUMBER_CONFIDENCE_THRESHOLD` | `0.5` | Min confidence to classify (0-1) |
| `LUMBER_DEDUP_WINDOW` | `5s` | Dedup window duration (`0` disables) |
| `LUMBER_MAX_BUFFER_SIZE` | `1000` | Max events buffered before flush |

</details>

<details>
<summary><strong>Output settings</strong></summary>

| Variable | Default | Description |
|---|---|---|
| `LUMBER_OUTPUT_FILE` | - | NDJSON file output path |
| `LUMBER_OUTPUT_FILE_MAX_SIZE` | `0` | File rotation size in bytes (0 = no rotation) |
| `LUMBER_WEBHOOK_URL` | - | Webhook HTTP POST endpoint |
| `LUMBER_WEBHOOK_HEADER_*` | - | Custom headers, e.g. `LUMBER_WEBHOOK_HEADER_AUTHORIZATION` |

Multiple outputs run simultaneously. File and webhook are async and won't stall the pipeline.

</details>

<details>
<summary><strong>Operational settings</strong></summary>

| Variable | Default | Description |
|---|---|---|
| `LUMBER_LOG_LEVEL` | `info` | Internal log level: `debug`, `info`, `warn`, `error` |
| `LUMBER_SHUTDOWN_TIMEOUT` | `10s` | Max drain time on shutdown |
| `LUMBER_POLL_INTERVAL` | provider default | Polling interval for stream mode |

</details>

### Verbosity levels

| Level | Behavior |
|---|---|
| `minimal` | Raw logs truncated to 200 characters |
| `standard` | Raw logs truncated to 2000 characters |
| `full` | Complete raw logs preserved |

---

## Development

```bash
make download-model  # Fetch ONNX model + tokenizer from HuggingFace
make build           # Build binary to bin/lumber
make test            # Run all tests
make lint            # Run golangci-lint
make clean           # Remove build artifacts
```

<details>
<summary><strong>Project structure</strong></summary>

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

</details>

---

## Roadmap

- [x] Local ONNX embedding + 42-leaf taxonomy (100% accuracy on 153-entry corpus)
- [x] Connectors: Vercel, Fly.io, Supabase, stdin, file
- [x] Multi-output: stdout, file rotation, webhook with retry
- [x] Public Go library API with concurrent safety
- [x] Interactive setup wizard + model auto-download
- [x] Production hardening (thread safety, input validation, graceful shutdown)
- [ ] Additional connectors (AWS CloudWatch, Datadog, Grafana Loki)
- [ ] HTTP server mode
- [ ] Adaptive taxonomy (self-growing/trimming)
- [ ] Field extraction from unstructured text

See [docs/changelog.md](docs/changelog.md) for release notes and [docs/plans/post-beta-proposals.md](docs/plans/post-beta-proposals.md) for the full roadmap.

---

<p align="center">
  Built by <a href="https://github.com/kaminocorp">Kamino Corporation</a>&ensp;|&ensp;<a href="LICENSE">Apache 2.0</a>
</p>
