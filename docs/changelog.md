# Changelog

## Index

- [0.2.2](#022--2026-02-19) — Download projection layer weights for full 1024-dim embeddings
- [0.2.1](#021--2026-02-19) — ONNX Runtime integration: session lifecycle, raw inference, dynamic tensor discovery
- [0.2.0](#020--2026-02-19) — Model download pipeline: Makefile target, tokenizer config, vocab path
- [0.1.0](#010--2026-02-19) — Project scaffolding: module structure, pipeline skeleton, classifier, compactor, and default taxonomy

---

## 0.2.2 — 2026-02-19

**Embedding engine — projection layer download (plan Section 5 amendment)**

### Added

- `make download-model` now fetches the sentence-transformers `2_Dense` projection layer from the official `MongoDB/mdbr-leaf-mt` repo:
  - `2_Dense/model.safetensors` (1.57MB) — `[1024, 384]` weight matrix
  - `2_Dense/config.json` — confirms: `in_features: 384`, `out_features: 1024`, `bias: false`, identity activation
- `OFFICIAL_BASE` URL variable in Makefile pointing to `MongoDB/mdbr-leaf-mt` (separate from `onnx-community` used for the ONNX model)
- `.gitignore` — added `/models/2_Dense/`

### Discovered

- The ONNX export (both official and community repos) only contains the base transformer (stage 1 of 3). The full mdbr-leaf-mt sentence-transformers pipeline is:
  1. **Transformer** (ONNX) → `[batch, seq, 384]` per-token hidden states
  2. **Mean pooling** (not in ONNX) → `[batch, 384]`
  3. **Dense projection** (not in ONNX) → `[batch, 1024]` via linear layer, no bias
- The plan's 1024-dim target was correct all along — the projection must be applied in Go after mean pooling (Section 3)

### Files changed

- `Makefile` — added `OFFICIAL_BASE`, projection layer download block
- `.gitignore` — added `2_Dense/` pattern

---

## 0.2.1 — 2026-02-19

**Embedding engine — ONNX Runtime integration (plan Section 1)**

### Added

- `onnxruntime-go` v1.26.0 dependency — pre-compiled `libonnxruntime.so` for aarch64 Linux ships with the package
- `internal/engine/embedder/onnx.go` — ONNX session wrapper:
  - Process-wide singleton runtime init via `sync.Once`
  - `DynamicAdvancedSession` for variable batch sizes at runtime
  - Auto-discovers input/output tensor names and embedding dimension from the model
  - Validates expected BERT-style inputs (`input_ids`, `attention_mask`, `token_type_ids`)
  - Raw `infer()` method: takes flat int64 slices, returns flat float32 output
  - Session options: 4 intra-op threads, sequential inter-op execution
- `Close() error` added to `Embedder` interface (embeds cleanup responsibility into the contract)
- `ONNXEmbedder.EmbedDim()` method — exposes model's embedding dimension
- `internal/engine/embedder/onnx_test.go` — 3 integration tests (session load, single inference, batch inference)

### Changed

- `ONNXEmbedder.New()` now loads the real ONNX model and creates an inference session (fails fast if model missing/corrupt)
- `ONNXEmbedder` struct holds `*onnxSession` instead of a bare path
- `cmd/lumber/main.go` — `defer emb.Close()` after embedder creation
- `Makefile` `download-model` target now also copies `libonnxruntime.so` from the Go module cache
- `.gitignore` — added `libonnxruntime.so`
- Default model path changed to `models/model_quantized.onnx` (preserves original ONNX filename so external data reference resolves)

### Discovered

- ONNX export (both official and community) outputs **384-dim** per-token hidden states from the base transformer. The final **1024-dim** embeddings require post-processing in Go: mean pooling → dense projection via `2_Dense/model.safetensors` (`[1024, 384]` linear, no bias). The projection weights (~1.57MB) need to be downloaded separately from the official `MongoDB/mdbr-leaf-mt` repo. The ONNX output dimension (384) is discovered dynamically by the code.
- ONNX Runtime `cpuid_info` warning on aarch64 (`Unknown CPU vendor`) is harmless — inference works correctly.

### Stubbed (not yet functional)

- `ONNXEmbedder.Embed` / `EmbedBatch` — needs tokenizer (Section 2) and mean pooling (Section 3)
- Taxonomy label pre-embedding — depends on working embedder

---

## 0.2.0 — 2026-02-19

**Embedding engine — model download pipeline (plan Section 5)**

### Added

- `make download-model` fetches from `onnx-community/mdbr-leaf-mt-ONNX` on HuggingFace:
  - `model_quantized.onnx` (216KB graph) + `model_quantized.onnx_data` (22MB int8 weights)
  - `vocab.txt` (227KB, 30,522 WordPiece tokens)
  - `tokenizer_config.json` (confirms: `BertTokenizer`, `do_lower_case: true`, `max_length: 128`)
- Idempotent download — skips if all key files already present
- `VocabPath` field in `EngineConfig` with env var `LUMBER_VOCAB_PATH` (default: `models/vocab.txt`)

### Changed

- `.gitignore` — added patterns for `*.onnx_data`, `vocab.txt`, `tokenizer_config.json`

### Design decisions

- **Int8 quantized over fp32:** 23MB vs 92MB, 4x smaller, faster on CPU. Log classification doesn't need fp32 precision. Swappable via a one-line URL change in the Makefile.
- **Original filenames preserved:** ONNX models hardcode external data file references internally — renaming breaks the reference. Files remain `model_quantized.onnx` / `model_quantized.onnx_data`.

---

## 0.1.0 — 2026-02-19

**Scaffolding — full project skeleton**

### Added

- Go module (`github.com/crimson-sun/lumber`, Go 1.23) with Makefile (build, test, lint, clean, download-model)
- `RawLog`, `CanonicalEvent`, `TaxonomyNode`, and `EmbeddedLabel` domain types
- `Connector` interface with provider registry and self-registering Vercel stub
- `Embedder` interface with `ONNXEmbedder` stub (awaiting ONNX runtime integration)
- Taxonomy manager with default taxonomy: 8 categories, 34 leaf labels (ERROR, REQUEST, DEPLOY, SYSTEM, SECURITY, DATA, SCHEDULED, APPLICATION)
- Cosine-similarity classifier with confidence threshold (fully implemented, no external deps)
- Token-aware compactor with 3 verbosity levels (full, moderate, compact)
- Engine orchestrator wiring embed → classify → compact
- `Output` interface with JSON-to-stdout implementation
- Pipeline connecting connector → engine → output (stream and query modes)
- Env-based config loader with defaults
- CLI entrypoint with graceful shutdown

### Stubbed (not yet functional)

- `ONNXEmbedder.Embed` / `EmbedBatch` — needs `onnxruntime-go`
- `vercel.Connector.Stream` / `Query` — needs Vercel API client
- Taxonomy label pre-embedding — depends on working embedder
