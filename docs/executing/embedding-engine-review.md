# Embedding Engine — Post-Implementation Review

Review of sections 1–5 (ONNX runtime, tokenizer, pooling/projection, taxonomy pre-embedding, model download).

---

## Status

All 22 tests pass. The embedding pipeline is functional end-to-end: `Embed("any log line")` returns a real 1024-dim vector, `EmbedBatch` processes multiple texts in a single ONNX inference call, and taxonomy labels are pre-embedded at startup.

---

## What's solid

**Component isolation.** Each pipeline stage is a self-contained unit (`onnx.go`, `tokenizer.go`, `pool.go`, `projection.go`) composed in `embedder.go`. The `Embedder` interface is narrow (3 methods). Components fail fast at init time with dimension validation (`session.embedDim == projection.inDim`).

**Tokenizer fidelity.** Faithful BERT WordPiece implementation (~290 lines, pure Go). Character classification matches BERT's Python `BasicTokenizer` exactly, including the wider ASCII punctuation range. Reference-validated token-for-token against HuggingFace output. Batch padding to longest-in-batch (not maxSeqLen) is a good optimization for inference.

**Zero-dependency safetensors loader.** Pure-stdlib, ~60 lines. Reads header, validates dtype/shape, extracts weights. No third-party library for a one-time load.

**Mean pooling correctness.** Correctly masks out padding positions. Handles the all-padding edge case (zero vector).

**ONNX session lifecycle.** Singleton init via `sync.Once`, `DynamicAdvancedSession` for variable batch sizes, proper `defer Destroy()` on tensors, data copy before tensor destruction.

---

## Issues

### 1. `ProcessBatch` doesn't use `EmbedBatch`

**File:** `internal/engine/engine.go:61-71`
**Severity:** Performance

`ProcessBatch` loops over raws and calls `Process` individually — N separate ONNX inference calls instead of one batched call. This defeats the purpose of `EmbedBatch`. The hot path for streaming logs will hit this.

**Fix:** Extract the embedding step to call `EmbedBatch` once for the full batch, then classify/compact per-event using the returned vectors.

### 2. Custom `sqrt` instead of `math.Sqrt`

**File:** `internal/engine/classifier/classifier.go:60-68`
**Severity:** Code quality

Newton's method with 64 iterations instead of `math.Sqrt`. There's no reason to avoid the stdlib — `math.Sqrt` compiles to a single CPU instruction on all Go platforms. The custom version is slower and harder to read.

**Fix:** Replace with `math.Sqrt`.

### 3. `inferSeverity` is incomplete

**File:** `internal/engine/engine.go:73-84`
**Severity:** Correctness

Only maps ERROR→error, SECURITY→warning, DEPLOY→info, default→info. The taxonomy has categories where this produces misleading severity: a `REQUEST.server_error` (5xx) gets "info", a `PERFORMANCE.latency_spike` gets "info", a `SCHEDULED.cron_failed` gets "info".

**Fix:** Severity should account for the leaf category, not just the top-level type. Options:
- Add a `Severity` field to `TaxonomyNode` so each leaf declares its own severity
- Derive from both type and category with a more complete mapping
- Keep the top-level mapping but add category-level overrides for error-like leaves

### 4. `Embed` always infers at maxSeqLen (128)

**File:** `internal/engine/embedder/embedder.go:59`
**Severity:** Performance (minor)

`Embed()` always runs inference on 128 positions even for short inputs. `EmbedBatch` gets dynamic padding via `tokenizeBatch`, but the single-text path doesn't. For the per-log-line hot path this means unnecessary computation on padding tokens.

**Fix:** Add a `tokenizeSingle` variant that pads to the actual sequence length, or route single-text embed through `tokenizeBatch` with a 1-element batch.

### 5. No L2 normalization of final embeddings

**File:** `internal/engine/embedder/embedder.go:65, 90`
**Severity:** Design (minor, not a bug)

Sentence-transformers models typically L2-normalize output embeddings. The cosine similarity in the classifier handles unnormalized vectors correctly (divides by norms), so classification works. But if embeddings are ever used outside the classifier (similarity thresholds, clustering for adaptive taxonomy, caching), pre-normalizing would make dot product equivalent to cosine — cheaper and more conventional.

**Fix (deferred):** Add optional L2 normalization as the final step in `Embed`/`EmbedBatch`. Not urgent — cosine similarity handles it.

### 6. Makefile `test` target doesn't set `LD_LIBRARY_PATH`

**File:** `Makefile:12`
**Severity:** Fragility

`make test` runs `go test ./...` without `LD_LIBRARY_PATH=models`. Tests currently work because `newONNXSession` derives the library path from `filepath.Dir(modelPath)` and tests use relative paths. This holds as long as tests run from the repo root, but breaks if run from another directory.

**Fix:** Set `LD_LIBRARY_PATH` in the Makefile test target, or document the requirement.

---

## Minor notes

- `tokenizeBatch` calls `tokenize` per-text (allocates 3x128-len slices each), then re-packs into trimmed slices. Double allocation for large batches. Not a concern at current scale.
- `embedder.Embedder` is slightly stuttery as a qualified name. Cosmetic only.
- Completion notes reference test counts that have drifted from actual counts. Minor doc staleness.

---

## Recommended fix order

1. **`ProcessBatch` batching** — performance on the hot path
2. **`math.Sqrt`** — trivial fix, no reason to keep custom impl
3. **`inferSeverity`** — correctness for non-ERROR categories
4. **Single-text padding** — performance optimization
5. **Makefile `LD_LIBRARY_PATH`** — fragility fix
6. **L2 normalization** — defer until adaptive taxonomy work
