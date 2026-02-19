# Lumber

A high-performance log normalization pipeline written in Go. Raw logs go in — from any provider, any format. Structured, canonical, token-efficient events come out.

```
Raw logs (Vercel, AWS, Fly.io, Datadog, …)
   ↓  connectors
Embed → Classify → Canonicalize → Compact
   ↓  engine
Structured canonical events (JSON)
```

## Why

Every log provider has a different API, auth mechanism, and response format. Every application logs differently. Lumber normalizes all of it into a single schema using a local embedding model and semantic classification — no cloud API calls, no LLM dependency.

This matters most for AI agent workflows that consume logs. Raw log dumps waste tokens, break on inconsistent formats, and require per-source integration code. Lumber solves that.

## How It Works

1. **Connectors** ingest raw logs from providers (Vercel, AWS, etc.) via a unified interface
2. **Embedder** converts each log line into a vector using a local ONNX model (~23MB, CPU-only)
3. **Classifier** compares the vector against pre-embedded taxonomy labels using cosine similarity
4. **Compactor** strips noise and truncates for token efficiency

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
  "confidence": 0.91,
  "raw": "ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)"
}
```

## Quickstart

### Prerequisites

- Go 1.23+
- `curl` (for model download)

### Setup

```bash
git clone https://github.com/crimson-sun/lumber.git
cd lumber

# Download the embedding model (~23MB) and ONNX runtime library
make download-model

# Build
make build
```

### Run

```bash
# Set your provider credentials
export LUMBER_CONNECTOR=vercel
export LUMBER_API_KEY=your-token-here

# Start streaming
./bin/lumber
```

Lumber reads configuration from environment variables:

| Variable | Default | Description |
|---|---|---|
| `LUMBER_CONNECTOR` | `vercel` | Log provider to connect to |
| `LUMBER_API_KEY` | — | Provider API key/token |
| `LUMBER_ENDPOINT` | — | Provider-specific endpoint URL |
| `LUMBER_MODEL_PATH` | `models/model_quantized.onnx` | Path to ONNX model file |
| `LUMBER_VOCAB_PATH` | `models/vocab.txt` | Path to tokenizer vocabulary |
| `LUMBER_VERBOSITY` | `standard` | Output verbosity: `minimal`, `standard`, `full` |
| `LUMBER_OUTPUT` | `stdout` | Output destination |

### Verbosity levels

- **minimal** — raw logs truncated to 200 characters
- **standard** — raw logs truncated to 2000 characters
- **full** — complete raw logs preserved

## Taxonomy

Lumber ships with a curated taxonomy of ~40 leaf labels organized under 8 top-level categories:

| Category | Labels |
|---|---|
| **ERROR** | runtime_exception, connection_failure, timeout, auth_failure, validation_error |
| **REQUEST** | incoming_request, outgoing_request, response |
| **DEPLOY** | build_started, build_succeeded, build_failed, deploy_started, deploy_succeeded, deploy_failed |
| **SYSTEM** | startup, shutdown, health_check, resource_limit, scaling |
| **SECURITY** | login_success, login_failure, rate_limited, suspicious_activity |
| **DATA** | query, migration, cache_hit, cache_miss |
| **SCHEDULED** | cron_started, cron_completed, cron_failed |
| **APPLICATION** | info, warning, debug |

Every log is classified into exactly one leaf label. The taxonomy is opinionated by design — a finite set of labels makes downstream consumption predictable.

## Project Structure

```
cmd/lumber/          CLI entrypoint
internal/
  config/            Environment-based configuration
  connector/         Provider adapters (Connector interface + registry)
  engine/
    embedder/        ONNX Runtime embedding (Embedder interface)
    classifier/      Cosine similarity classification
    compactor/       Token-aware log compaction
    taxonomy/        Taxonomy tree and default labels
  model/             Domain types (RawLog, CanonicalEvent, TaxonomyNode)
  output/            Output writers (Output interface)
  pipeline/          Stream and Query orchestration
models/              ONNX model files (downloaded via make)
```

## Development

```bash
make build           # Build binary to bin/lumber
make test            # Run all tests
make lint            # Run golangci-lint
make clean           # Remove build artifacts
make download-model  # Fetch ONNX model + tokenizer from HuggingFace
```

## Embedding Model

Lumber uses [MongoDB LEAF (mdbr-leaf-mt)](https://huggingface.co/onnx-community/mdbr-leaf-mt-ONNX), a 23M parameter text embedding model. It runs locally via ONNX Runtime — no external API calls, no GPU required.

- **Size:** ~23MB (int8 quantized)
- **Embedding dimension:** 384
- **Tokenizer:** WordPiece (30,522 tokens, lowercase)
- **Runtime:** ONNX Runtime via [onnxruntime-go](https://github.com/yalue/onnxruntime_go)

## Status

Lumber is under active development. Current state:

- [x] Project scaffolding and pipeline skeleton
- [x] Model download pipeline
- [x] ONNX Runtime session lifecycle and raw inference
- [ ] Tokenizer integration
- [ ] Mean pooling and end-to-end embedding
- [ ] Taxonomy label pre-embedding
- [ ] Connector implementations (Vercel, AWS, etc.)
- [ ] Output formats beyond stdout

See [docs/changelog.md](docs/changelog.md) for detailed release notes.

## License

[Apache 2.0](LICENSE)
