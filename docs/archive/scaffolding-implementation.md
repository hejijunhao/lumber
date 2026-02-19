# Scaffolding Implementation Plan

## Overview

This plan defines the initial directory structure and file layout for Lumber. The goal is to establish Go project conventions, clear module boundaries matching the three-layer architecture (Connectors → Classification Engine → Output), and a foundation that supports incremental development.

---

## Directory Structure

```
lumber/
├── cmd/
│   └── lumber/
│       └── main.go                  # Binary entrypoint — wires up config, starts pipeline
│
├── internal/
│   ├── config/
│   │   └── config.go                # Configuration loading (env, file, defaults)
│   │
│   ├── connector/
│   │   ├── connector.go             # Connector interface definition (Stream, Query)
│   │   ├── registry.go              # Connector registry — lookup by provider name
│   │   └── vercel/
│   │       └── vercel.go            # First connector implementation (Vercel REST)
│   │
│   ├── engine/
│   │   ├── engine.go                # Classification engine — orchestrates embed→classify→canonicalize→compact
│   │   ├── embedder/
│   │   │   └── embedder.go          # Embedding interface + ONNX runtime wrapper
│   │   ├── taxonomy/
│   │   │   ├── taxonomy.go          # Taxonomy tree: loading, lookup, pre-embedding labels
│   │   │   └── default.go           # Default taxonomy definition (the ~40-50 labels from vision)
│   │   ├── classifier/
│   │   │   └── classifier.go        # Cosine similarity classification against taxonomy vectors
│   │   └── compactor/
│   │       └── compactor.go         # Token-aware compaction: truncation, dedup, verbosity levels
│   │
│   ├── model/
│   │   ├── rawlog.go                # RawLog type — what connectors produce
│   │   ├── event.go                 # CanonicalEvent type — what the engine produces
│   │   └── taxonomy.go              # Taxonomy node/label types
│   │
│   ├── output/
│   │   ├── output.go                # Output interface definition
│   │   └── stdout/
│   │       └── stdout.go            # Simplest output: write canonical events to stdout (for dev/debug)
│   │
│   └── pipeline/
│       └── pipeline.go              # Pipeline orchestration — connects connectors → engine → output
│
├── models/                          # ONNX model files (gitignored, downloaded at build/setup)
│   └── .gitkeep
│
├── docs/
│   ├── vision.md
│   ├── changelog.md
│   └── plans/
│       └── scaffolding-implementation.md   # This file
│
├── go.mod                           # Go module definition
├── go.sum                           # Go dependency checksums (generated)
├── .gitignore
├── Makefile                         # Build, test, lint, model download targets
└── LICENSE
```

---

## File Purposes & Contents

### `cmd/lumber/main.go`

Binary entrypoint. Responsibilities:
- Parse CLI flags / load config
- Initialize the pipeline (connector → engine → output)
- Start the pipeline, block until shutdown signal (SIGINT/SIGTERM)
- Graceful shutdown

Keeps main thin — all real logic lives in `internal/`.

### `internal/config/config.go`

Configuration struct and loading logic.
- Defines `Config` struct covering: connector settings, engine settings (model path, taxonomy), output settings, verbosity level
- Loads from environment variables with sensible defaults
- File-based config (YAML/TOML) can be added later; env vars are sufficient for v1

### `internal/connector/connector.go`

Defines the core connector interfaces from the vision doc:

```go
type Connector interface {
    Stream(ctx context.Context, cfg ConnectorConfig) (<-chan RawLog, error)
    Query(ctx context.Context, cfg ConnectorConfig, params QueryParams) ([]RawLog, error)
}
```

Also defines `ConnectorConfig` and `QueryParams` types.

### `internal/connector/registry.go`

A simple registry mapping provider names (strings) to connector constructors. Allows config-driven connector selection without import-time coupling.

### `internal/connector/vercel/vercel.go`

First concrete connector. Implements the `Connector` interface for Vercel's log drain / REST API. Handles auth, pagination, response parsing into `RawLog`.

### `internal/engine/engine.go`

The classification engine — the core of Lumber. Orchestrates the four-step pipeline:
1. Receive `RawLog`
2. Embed via `embedder`
3. Classify via `classifier` (against taxonomy)
4. Compact via `compactor`
5. Return `CanonicalEvent`

Exposes a `Process(RawLog) (CanonicalEvent, error)` method and a batch variant.

### `internal/engine/embedder/embedder.go`

Embedding interface and ONNX Runtime implementation:

```go
type Embedder interface {
    Embed(text string) ([]float32, error)
    EmbedBatch(texts []string) ([][]float32, error)
}
```

Wraps `onnxruntime-go`. Handles model loading, tokenization, inference. This is the only file that touches the ONNX runtime directly.

### `internal/engine/taxonomy/taxonomy.go`

Taxonomy management:
- Load taxonomy tree (from `default.go` or external config)
- Pre-embed all leaf labels at startup using the embedder
- Provide lookup: given a category path, return the label + its vector
- Expose the full set of pre-embedded labels for the classifier

### `internal/engine/taxonomy/default.go`

The default taxonomy definition — the ~40-50 labels from the vision doc, defined as a Go data structure. Serves as the built-in taxonomy that ships with Lumber.

### `internal/engine/classifier/classifier.go`

Classification logic:
- Takes a log embedding vector + the set of pre-embedded taxonomy labels
- Computes cosine similarity against all labels
- Returns the best match (label + confidence score)
- Handles confidence thresholds (below threshold → "unclassified")

Pure computation — no I/O, no side effects.

### `internal/engine/compactor/compactor.go`

Token-aware compaction:
- Truncate verbose fields (stack traces, request bodies) based on verbosity level
- Strip redundant metadata
- Deduplicate repeated events into counted summaries
- Three verbosity modes: minimal / standard / full

### `internal/model/rawlog.go`

The `RawLog` struct — the intermediate type between connectors and the engine:

```go
type RawLog struct {
    Timestamp  time.Time
    Source     string          // provider name
    Raw        string          // original log text
    Metadata   map[string]any  // provider-specific metadata
}
```

### `internal/model/event.go`

The `CanonicalEvent` struct — Lumber's output type:

```go
type CanonicalEvent struct {
    Type       string    // top-level category (ERROR, REQUEST, DEPLOY, etc.)
    Category   string    // leaf label (connection_failure, build_succeeded, etc.)
    Severity   string    // normalized severity
    Timestamp  time.Time
    Summary    string    // human-readable summary
    Confidence float64   // classification confidence score
    Raw        string    // original log text (retained at standard/full verbosity)
}
```

### `internal/model/taxonomy.go`

Types for the taxonomy tree:

```go
type TaxonomyNode struct {
    Name     string
    Children []*TaxonomyNode
    Desc     string          // description used for embedding
}

type EmbeddedLabel struct {
    Path   string      // e.g. "ERROR.connection_failure"
    Vector []float32
}
```

### `internal/output/output.go`

Output interface:

```go
type Output interface {
    Write(ctx context.Context, event CanonicalEvent) error
    Close() error
}
```

Additional outputs (gRPC, WebSocket, webhook) will implement this same interface later.

### `internal/output/stdout/stdout.go`

Simplest output implementation — writes JSON-encoded canonical events to stdout. Used for development, debugging, and CLI piping.

### `internal/pipeline/pipeline.go`

Pipeline orchestration — the glue:
- Accepts a `Connector`, `Engine`, and `Output`
- In stream mode: reads from connector channel, processes through engine, writes to output
- In query mode: fetches logs, processes batch, returns results
- Handles goroutine lifecycle, error propagation, graceful shutdown

### `models/.gitkeep`

Placeholder for ONNX model files. The actual model binaries are downloaded via `make download-model` (not checked into git).

### `go.mod`

```
module github.com/crimson-sun/lumber
go 1.23
```

Module path TBD — using a placeholder. No dependencies added yet beyond the standard library; `onnxruntime-go` will be added when the embedder is implemented.

### `.gitignore`

```
# Binaries
/bin/
/lumber

# Models (downloaded, not committed)
/models/*.onnx
/models/*.bin

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store

# Go
/vendor/
```

### `Makefile`

Targets:
- `build` — compile the binary to `bin/lumber`
- `test` — run all tests
- `lint` — run `golangci-lint`
- `download-model` — fetch the ONNX model to `models/`
- `clean` — remove build artifacts

---

## Implementation Steps

1. **Create `.gitignore`** and **`go.mod`**
2. **Create `internal/model/`** — define `RawLog`, `CanonicalEvent`, taxonomy types (these are dependency-free and everything else imports them)
3. **Create `internal/connector/`** — interface + registry + Vercel stub
4. **Create `internal/engine/`** — engine orchestrator + sub-package stubs (embedder, taxonomy, classifier, compactor)
5. **Create `internal/output/`** — interface + stdout implementation
6. **Create `internal/pipeline/`** — pipeline orchestration
7. **Create `cmd/lumber/main.go`** — wire everything together
8. **Create `internal/config/`** — config loading
9. **Create `Makefile`**
10. **Create `models/.gitkeep`**

Each step produces compilable Go code. Stubs will have the correct interfaces and type signatures but minimal implementation (returning zero values or `ErrNotImplemented`). The goal is a skeleton that compiles, runs, and clearly communicates where each piece of functionality will live.

---

## Notes

- Everything under `internal/` is unexported to consumers — this is intentional. If we later want Lumber to be importable as a library, we'll create a top-level `lumber` package that exposes a curated public API.
- Connector implementations each get their own sub-package (`vercel/`, later `aws/`, `flyio/`, etc.) to keep provider-specific dependencies isolated.
- The engine sub-packages (embedder, taxonomy, classifier, compactor) are separated because they have distinct responsibilities and will be tested independently, but they're all coordinated by `engine.go`.
- No `pkg/` directory — we don't have public library APIs yet. If needed later, we'll add it deliberately.
