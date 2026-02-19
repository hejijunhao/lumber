# Changelog

## Index

- [0.2.6](#026--2026-02-19) — Post-review fixes: batched inference, leaf severity, dynamic padding, math.Sqrt
- [0.2.5](#025--2026-02-19) — Taxonomy pre-embedding: batch embed all 34 leaf labels at startup
- [0.2.4](#024--2026-02-19) — Mean pooling + dense projection: full 1024-dim embeddings, end-to-end Embed/EmbedBatch
- [0.2.3](#023--2026-02-19) — Pure-Go WordPiece tokenizer: vocab loader, BERT tokenization, batch packing
- [0.2.2](#022--2026-02-19) — Download projection layer weights for full 1024-dim embeddings
- [0.2.1](#021--2026-02-19) — ONNX Runtime integration: session lifecycle, raw inference, dynamic tensor discovery
- [0.2.0](#020--2026-02-19) — Model download pipeline: Makefile target, tokenizer config, vocab path
- [0.1.0](#010--2026-02-19) — Project scaffolding: module structure, pipeline skeleton, classifier, compactor, and default taxonomy

---

## 0.2.6 — 2026-02-19

**Embedding engine — post-review fixes (plan Section 6)**

### Changed

- `ProcessBatch` now calls `EmbedBatch` once for the full batch instead of looping `Process` per event — single ONNX inference call instead of N
- `Embed()` routes through `tokenizeBatch` with a 1-element slice, giving single-text inference the same dynamic-padding-to-longest behavior as `EmbedBatch` — a 10-token log line now infers on ~12 positions instead of 128
- Replaced custom 64-iteration Newton's method `sqrt` with `math.Sqrt` in cosine similarity — compiles to a single CPU instruction
- Severity now comes from per-leaf `Severity` field on `EmbeddedLabel` instead of `inferSeverity()` which only mapped top-level types — fixes incorrect severity for leaves like `DEPLOY.build_failed` (was "info", now "error") and `SCHEDULED.cron_failed`
- `Makefile` test target prefixed with `LD_LIBRARY_PATH=$(MODEL_DIR)` for reliable test execution outside repo root

### Added

- `Severity string` field on `TaxonomyNode` and `EmbeddedLabel`
- Severity set on every leaf in `DefaultRoots()`: all ERROR children → error (except `validation_error` → warning), `build_failed`/`deploy_failed`/`cron_failed` → error, security leaves → warning, `cache_hit`/`debug` → debug, everything else → info

### Removed

- `inferSeverity()` function in `engine.go`
- Custom `sqrt()` function in `classifier.go`

### Deferred

- L2 normalization of final embeddings — not a bug (cosine similarity handles unnormalized vectors), deferred until adaptive taxonomy work where embeddings may be used outside the classifier

### Files changed

- `internal/engine/engine.go` — batched `ProcessBatch`, removed `inferSeverity`
- `internal/engine/classifier/classifier.go` — `math.Sqrt` replacement
- `internal/model/taxonomy.go` — added `Severity` to both structs
- `internal/engine/taxonomy/default.go` — severity on every leaf
- `internal/engine/taxonomy/taxonomy.go` — propagate severity to embedded labels
- `internal/engine/taxonomy/taxonomy_test.go` — severity in test fixtures and assertions
- `internal/engine/embedder/embedder.go` — `Embed()` dynamic padding
- `Makefile` — `LD_LIBRARY_PATH` in test target

---

## 0.2.5 — 2026-02-19

**Embedding engine — taxonomy pre-embedding (plan Section 4)**

### Added

- `taxonomy.New(roots, embedder)` now pre-embeds all leaf labels at startup via a single `EmbedBatch` call:
  - Walks roots → children, builds embedding texts as `"{Parent}: {Leaf.Desc}"` (e.g., `"ERROR: Network or database connection failure"`)
  - Paths stored as `"ERROR.connection_failure"` for classifier consumption
  - Edge cases: empty roots or roots with no children short-circuit before calling the embedder
- Startup logging in `main.go` — logs model path with `dim=1024` after embedder init, label count and wall-clock duration after taxonomy init (e.g., `pre-embedded 34 labels in 142ms`)
- `internal/engine/taxonomy/taxonomy_test.go` — 4 tests using mock embedder:
  - `TestNewPreEmbeds` — correct paths, vector dimensions, and non-zero values
  - `TestNewEmptyRoots` — nil roots → 0 labels, embedder never called
  - `TestNewNoLeaves` — root-only nodes → 0 labels, embedder never called
  - `TestNewEmbedError` — embedder failure propagates as wrapped error

### Design decisions

- **Embedding text format `"{Parent}: {Leaf.Desc}"`** — gives the model both category context and semantic description; the dotted path is a code identifier, not useful for embedding
- **Single `EmbedBatch` call** — one ONNX inference pass for all 34 labels keeps startup fast (~100-300ms)

### Files changed

- `internal/engine/taxonomy/taxonomy.go` — replaced stub with leaf collection + batch embedding
- `internal/engine/taxonomy/taxonomy_test.go` — **new**, 4 unit tests
- `cmd/lumber/main.go` — startup logging

---

## 0.2.4 — 2026-02-19

**Embedding engine — mean pooling + dense projection (plan Section 3)**

### Added

- `internal/engine/embedder/projection.go` — safetensors loader + linear projection:
  - Parses safetensors binary format using only `encoding/binary` + `encoding/json` (no new deps, ~60 lines)
  - Loads `"linear.weight"` tensor, validates dtype=`F32` and shape=`[1024, 384]`
  - `apply(vec)` — matrix-vector multiply projecting 384-dim → 1024-dim
- `internal/engine/embedder/pool.go` — attention-mask-weighted mean pooling:
  - Averages hidden states only at positions where `mask == 1`
  - All-padding sequences produce zero vectors (no divide-by-zero)
- `ProjectionPath` in `EngineConfig` with env var `LUMBER_PROJECTION_PATH` (default: `models/2_Dense/model.safetensors`)
- `internal/engine/embedder/pool_test.go` — 3 tests (single sample, batch, all-padding)
- `internal/engine/embedder/projection_test.go` — 5 tests:
  - `TestLoadProjection` — real safetensors, shape `[1024, 384]`, non-zero weights
  - `TestProjectionApply` — uniform input → 1024-dim non-zero output
  - `TestEmbedEndToEnd` — `Embed("hello world")` → 1024-dim vector, `EmbedDim() == 1024`
  - `TestEmbedBatchEndToEnd` — 2 texts → distinct 1024-dim vectors
  - `TestEmbedBatchEmpty` — nil → nil

### Changed

- `ONNXEmbedder` struct now holds `*onnxSession`, `*tokenizer`, and `*projection`
- `New(modelPath, vocabPath, projectionPath)` — loads all three, validates `session.embedDim == projection.inDim` at construction (fails fast on mismatch), cleans up on partial failure
- `Embed(text)` — full pipeline: tokenize → infer → mean pool → project → 1024-dim vector
- `EmbedBatch(texts)` — tokenize batch → single infer → mean pool → project each → `[][]float32`
- `EmbedDim()` — now returns `projection.outDim` (1024) instead of `session.embedDim` (384)

### Design decisions

- **Pure-stdlib safetensors parsing** — format is simple enough that no third-party library is needed
- **Dimension validation at init** — catches model/projection mismatch immediately rather than producing garbage at runtime

### Files changed

- `internal/engine/embedder/projection.go` — **new**, safetensors loader + projection
- `internal/engine/embedder/pool.go` — **new**, mean pooling
- `internal/engine/embedder/pool_test.go` — **new**, 3 tests
- `internal/engine/embedder/projection_test.go` — **new**, 5 tests
- `internal/engine/embedder/embedder.go` — wired tokenizer + projection, implemented `Embed`/`EmbedBatch`
- `internal/config/config.go` — added `ProjectionPath`
- `cmd/lumber/main.go` — updated `embedder.New()` call

---

## 0.2.3 — 2026-02-19

**Embedding engine — WordPiece tokenizer (plan Section 2)**

### Added

- `internal/engine/embedder/vocab.go` — vocabulary loader:
  - Parses `vocab.txt` (one token per line, line number = token ID)
  - Bidirectional maps (`token→id`, `id→token`), 30,522 tokens
  - Validates and caches special token IDs: `[PAD]=0`, `[UNK]=100`, `[CLS]=101`, `[SEP]=102`
- `internal/engine/embedder/tokenizer.go` — full BERT tokenization pipeline:
  - Clean text (remove control chars, normalize whitespace) → CJK character padding → lowercase → strip accents (NFD + remove combining marks) → whitespace split → punctuation split → WordPiece (greedy longest-prefix with `##` continuation, 200-rune max per word) → wrap with `[CLS]`/`[SEP]` → truncate to 128 → right-pad to `maxSeqLen` → generate `attention_mask` and `token_type_ids`
  - `tokenizeBatch(texts)` — packs into flat slices padded to the *longest sequence in the batch* (not always 128), minimizing unnecessary ONNX computation
  - Character classification (`isPunctuation`, `isWhitespace`, `isControl`, `isChineseChar`) matches BERT's Python `BasicTokenizer` exactly
- `internal/engine/embedder/tokenizer_test.go` — 10 tests validated against HuggingFace `BertTokenizer` reference output:
  - `TestVocabLoad` — vocab size 30,522, all special token IDs
  - `TestTokenize` — 7 sub-tests: simple words, empty string, log line with punctuation/numbers, IP addresses, accented characters, CJK, mixed brackets
  - `TestTokenizeTruncation` — 200-word input → exactly 128 tokens
  - `TestTokenizeBatch` — flat packing, correct shape
  - `TestTokenizeBatchEmpty` — nil → zero batch
- `golang.org/x/text` v0.34.0 dependency (for `unicode/norm.NFD` accent stripping)

### Design decisions

- **Pure Go, no CGo tokenizer bindings** — WordPiece is simple enough (~250 lines), avoids HuggingFace Rust `tokenizers` dependency. Vocab is 30K entries — map lookup is fast.
- **Max sequence length 128** — log lines rarely exceed this; matches `tokenizer_config.json`. Shorter = faster inference.
- **Batch padding to longest sequence** — `tokenizeBatch` pads to the longest in the batch, not always 128. For typical 20-40 token log lines, this cuts unnecessary ONNX computation.

### Files changed

- `internal/engine/embedder/vocab.go` — **new**, vocabulary loader
- `internal/engine/embedder/tokenizer.go` — **new**, WordPiece tokenizer + batch tokenization
- `internal/engine/embedder/tokenizer_test.go` — **new**, 10 tests
- `go.mod`, `go.sum` — added `golang.org/x/text` v0.34.0

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
