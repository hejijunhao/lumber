# Section 3: Mean Pooling + Dense Projection — Completion Notes

## What Was Done

### 3.1 — Safetensors loader

Created `internal/engine/embedder/projection.go` with:

- **`loadProjection(path)`** — parses the safetensors binary format using only `encoding/binary` + `encoding/json` (no new dependencies). Reads the 8-byte LE uint64 header length, unmarshals the JSON metadata, locates the `"linear.weight"` tensor entry, validates dtype=`F32` and shape=`[1024, 384]`, then reads the raw float32 data from the correct file offset.
- **`projection` struct** — holds `weights []float32` (row-major `[outDim, inDim]`), `inDim` (384), `outDim` (1024).
- **`apply(vec)`** — matrix-vector multiply: for each output row, computes the dot product of the weight row with the input vector. Projects a 384-dim vector to 1024-dim.

### 3.2 — Mean pooling

Created `internal/engine/embedder/pool.go` with:

- **`meanPool(hidden, mask, batchSize, seqLen, dim)`** — attention-mask-aware averaging over the sequence dimension of transformer hidden states. For each sample in the batch: sums hidden states at positions where `mask == 1`, divides by the count of real (non-padding) tokens. All-padding sequences produce zero vectors.
- Input: flat `[batchSize * seqLen * dim]` hidden states + flat `[batchSize * seqLen]` attention mask.
- Output: flat `[batchSize * dim]` pooled vectors.

### 3.3 — End-to-end embedder wiring

Updated `internal/engine/embedder/embedder.go`:

- **`ONNXEmbedder`** struct now holds `*onnxSession`, `*tokenizer`, and `*projection`.
- **`New(modelPath, vocabPath, projectionPath)`** — loads all three components, validates that the ONNX output dimension (384) matches the projection input dimension (384). Cleans up on partial failure.
- **`Embed(text)`** — full pipeline: `tokenize(text)` → `infer(1, 128)` → `meanPool` → `projection.apply` → 1024-dim vector.
- **`EmbedBatch(texts)`** — `tokenizeBatch(texts)` → `infer(batchSize, seqLen)` → `meanPool` → `projection.apply` per sample → batch of 1024-dim vectors.
- **`EmbedDim()`** — now returns `projection.outDim` (1024) instead of `session.embedDim` (384).

### 3.4 — Config + main.go

- Added `ProjectionPath` to `EngineConfig` with env var `LUMBER_PROJECTION_PATH`, default `"models/2_Dense/model.safetensors"`.
- Updated `main.go` call site: `embedder.New(cfg.Engine.ModelPath, cfg.Engine.VocabPath, cfg.Engine.ProjectionPath)`.

### 3.5 — Tests

Created `internal/engine/embedder/pool_test.go` with 3 tests:

- **`TestMeanPool`** — single sample, 3 tokens (2 real + 1 padding), 2-dim hidden states; verifies averaged output
- **`TestMeanPoolBatch`** — 2 samples with different mask lengths; verifies per-sample averages
- **`TestMeanPoolAllPadding`** — all-padding mask produces zero vector

Created `internal/engine/embedder/projection_test.go` with 5 tests:

- **`TestLoadProjection`** — loads real safetensors file, verifies shape `[1024, 384]`, spot-checks weights aren't zero
- **`TestProjectionApply`** — uniform input vector, verifies 1024-dim output with non-zero values
- **`TestEmbedEndToEnd`** — `New()` → `Embed("hello world")` → verifies 1024-dim non-zero vector, `EmbedDim() == 1024`
- **`TestEmbedBatchEndToEnd`** — 2 semantically different texts, verifies both produce 1024-dim vectors that differ from each other
- **`TestEmbedBatchEmpty`** — nil input returns nil output

All 19 tests pass (3 ONNX session + 3 pool + 5 projection/e2e + 8 tokenizer).

## Design Decisions

### Pure-stdlib safetensors parsing

The safetensors format is simple: 8-byte header length, JSON metadata, raw tensor data. No third-party library needed — `encoding/binary` + `encoding/json` handle it in ~60 lines. This avoids adding a dependency for a one-time load operation.

### Mean pooling with attention mask

Mean pooling averages only over real (non-padding) token positions, matching the sentence-transformers `Pooling` module behavior. This is critical for correctness — including padding positions would dilute embeddings for short sequences.

### Dimension validation at init

`New()` validates `session.embedDim == projection.inDim` at construction time, failing fast if the ONNX model and projection weights are mismatched. This catches configuration errors immediately rather than producing garbage embeddings at runtime.

### No new dependencies

The entire Section 3 implementation uses only the standard library. The safetensors loader, mean pooling, and matrix-vector projection are all pure Go with no CGo or external packages.

## Dependencies Added

None.

## Files Changed

- `internal/engine/embedder/projection.go` — **new**, safetensors loader + projection apply
- `internal/engine/embedder/pool.go` — **new**, mean pooling
- `internal/engine/embedder/pool_test.go` — **new**, 3 pooling unit tests
- `internal/engine/embedder/projection_test.go` — **new**, 5 projection + end-to-end tests
- `internal/engine/embedder/embedder.go` — wired tokenizer + projection, implemented `Embed`/`EmbedBatch`
- `internal/config/config.go` — added `ProjectionPath`
- `cmd/lumber/main.go` — updated `embedder.New()` call

## Interface for Section 4

The embedder is now fully functional. Downstream consumers call:

- **`Embed(text) ([]float32, error)`** — single text → 1024-dim vector
- **`EmbedBatch(texts) ([][]float32, error)`** — batch → slice of 1024-dim vectors
- **`EmbedDim() int`** — returns 1024

The taxonomy manager can now pre-embed all leaf labels at startup, and the engine can embed incoming log lines for cosine-similarity classification.
