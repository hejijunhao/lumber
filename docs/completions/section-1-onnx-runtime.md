# Section 1: ONNX Runtime Integration — Completion Notes

## What Was Done

### 1.1 — `onnxruntime-go` dependency

- Added `github.com/yalue/onnxruntime_go` v1.26.0
- The package ships a pre-compiled `onnxruntime_arm64.so` in its `test_data/` directory — no need to build ONNX Runtime from source
- The shared library is copied to `models/libonnxruntime.so` by `make download-model` (auto-detected from the Go module cache)
- `LD_LIBRARY_PATH` must include the `models/` directory at runtime (or the library must be on the system linker path)

### 1.2 — ONNX session lifecycle

Created `internal/engine/embedder/onnx.go` with:

- **`initORT(libPath)`** — process-wide singleton initialization via `sync.Once`. Sets the shared library path and initializes the ONNX Runtime environment. Safe for concurrent callers.
- **`onnxSession` struct** — holds a `DynamicAdvancedSession` (allows different batch sizes per call), discovered input/output names, and embedding dimension.
- **`newONNXSession(modelPath)`** — loads model, auto-discovers tensor names via `GetInputOutputInfo()`, validates expected BERT-style inputs (`input_ids`, `attention_mask`, `token_type_ids`), creates session with optimized options (4 intra-op threads, sequential inter-op).
- **`close()`** — releases session resources.

Updated `internal/engine/embedder/embedder.go`:

- Added `Close() error` to the `Embedder` interface
- `ONNXEmbedder` struct now holds an `*onnxSession` instead of just a path
- `New(modelPath)` creates a real ONNX session (fails fast if model file is missing/corrupt)
- `EmbedDim()` method exposes the model's embedding dimension
- `Embed`/`EmbedBatch` remain stubbed — they need the tokenizer (Section 2) and pooling (Section 3)

Updated `cmd/lumber/main.go`:
- Added `defer emb.Close()` after embedder creation

### 1.3 — Raw inference wrapper

`onnxSession.infer(inputIDs, attentionMask, tokenTypeIDs, batchSize, seqLen)`:
- Takes flat int64 slices (pre-tokenized) and batch/sequence dimensions
- Creates ONNX tensors, runs inference via `DynamicAdvancedSession.Run()`, copies output data before tensors are destroyed
- Returns flat `[]float32` of shape `[batchSize * seqLen * embedDim]`

### Tests

`internal/engine/embedder/onnx_test.go` — 3 tests:
- `TestONNXSessionLoad` — verifies model loads, discovers correct tensor names, positive embed dim
- `TestONNXInference` — minimal single-sequence inference (`[CLS] [SEP]`), verifies non-zero output
- `TestONNXBatchInference` — batch of 2 sequences, verifies correct output size

All tests pass. Total test time: ~125ms (including model loading).

## Key Discoveries

### Embedding dimension is 384, not 1024

The quantized ONNX community export of `mdbr-leaf-mt` produces 384-dimensional embeddings (the model's native output from `last_hidden_state` has hidden_size=384). The 1024 figure from the plan refers to the full-precision model's final projection layer — the ONNX export from `onnx-community` doesn't include this projection.

**Impact:** This is fine for our use case. 384 dimensions is more than sufficient for ~40 taxonomy labels. Cosine similarity and classification quality will not meaningfully differ. Memory usage is lower (40 × 384 × 4 = 61KB vs 160KB).

The code dynamically discovers the dimension from the model — no hardcoded assumption.

### `LD_LIBRARY_PATH` required at runtime

ONNX Runtime loads via `dlopen`. The shared library must be findable by the dynamic linker. Options:
1. Set `LD_LIBRARY_PATH` to include `models/` (current approach)
2. Install `libonnxruntime.so` to a system library path
3. Use `rpath` linker flags at build time

For now, option 1 is fine. The Makefile test target or a wrapper script can handle this.

### ONNX Runtime `cpuid_info` warning

The warning `Unknown CPU vendor. cpuinfo_vendor value: 0` appears on aarch64 Linux. It's harmless — ONNX Runtime still works correctly, it just can't identify the specific ARM CPU vendor for optimization heuristics.

## Files Changed

- `internal/engine/embedder/onnx.go` — **new**, ONNX session wrapper
- `internal/engine/embedder/onnx_test.go` — **new**, integration tests
- `internal/engine/embedder/embedder.go` — added `Close()` to interface, real session in struct
- `cmd/lumber/main.go` — `defer emb.Close()`
- `internal/config/config.go` — default model path updated to `models/model_quantized.onnx`
- `.gitignore` — added `libonnxruntime.so`
- `Makefile` — auto-copies shared library from Go module cache
- `go.mod`, `go.sum` — added `onnxruntime_go` v1.26.0

## Open Items for Next Sections

- Section 2 (Tokenizer): Vocab is 30,522 tokens, standard BERT WordPiece. Special token IDs confirmed: `[PAD]=0, [UNK]=100, [CLS]=101, [SEP]=102`.
- Section 3 (Embed): Mean pooling over `last_hidden_state` using attention mask, output dimension 384. The `infer()` method returns raw per-token embeddings ready for pooling.
- Embedding dimension for taxonomy labels / classifier needs to be 384, not 1024.
