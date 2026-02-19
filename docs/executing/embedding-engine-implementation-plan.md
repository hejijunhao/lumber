# Phase 1: Embedding Engine — Implementation Plan

## Goal

Replace the stubbed `ONNXEmbedder` with a working local embedder that converts text to vectors using ONNX Runtime. After this phase, `Embed("any log line")` returns a real vector and taxonomy pre-embedding works end-to-end.

**Success criteria:**
- `Embed(text)` returns a real 1024-dimensional vector for any input string
- `EmbedBatch(texts)` processes multiple texts efficiently in a single inference call
- Taxonomy labels are pre-embedded at startup via `taxonomy.New()`
- `make download-model` fetches the model and vocab files
- Inference runs <10ms per log line on CPU
- Zero external network calls at runtime

---

## Model Selection: MongoDB mdbr-leaf-mt

**Why this model:**
- 22.6M parameters, 6 transformer layers — same speed class as all-MiniLM-L6-v2, ~1-3ms per inference on CPU
- Highest MTEB v2 (English) score for models ≤30M params (63.97), beating BGE-small-en-v1.5 (62.17) and all-MiniLM-L6-v2 (56.26) — models with 2x the layers and compute
- Distilled from `mxbai-embed-large-v1` using LEAF (Knowledge Distillation with Teacher-Aligned Representations), specifically optimized for classification, clustering, and STS — exactly our log classification use case
- 1024-dimensional embeddings at full fidelity. With only ~40 taxonomy labels, the memory (40 × 1024 × 4 bytes = 160KB) and cosine similarity compute are negligible
- Supports Matryoshka Representation Learning (MRL) for dimension truncation if needed later, but no reason to truncate at our scale
- WordPiece tokenizer with standard `vocab.txt` — same pure-Go tokenizer approach as any BERT-family model
- Apache 2.0 license
- Pre-built ONNX exports available on HuggingFace (`MongoDB/mdbr-leaf-mt` and `onnx-community/mdbr-leaf-mt-ONNX`)

**Models considered and rejected:**
- *all-MiniLM-L6-v2* — same speed but significantly lower quality (MTEB 56.26 vs 63.97). No reason to choose it when mdbr-leaf-mt exists at the same param count.
- *BGE-small-en-v1.5* — strong classification score (74.14) but 12 layers / 33.4M params = ~2x inference time for a lower overall MTEB score.
- *GTE-small* — 12 layers, lower MTEB than BGE-small. No advantage over mdbr-leaf-mt.
- *Snowflake arctic-embed-s* — retrieval-optimized, not classification/STS. Lower MTEB overall.
- *E5-small-v2* — requires mandatory query prefixes, lower scores than BGE/GTE.

**Files to download:**
- `model.onnx` (+ `model.onnx_data` if weights are external) — the ONNX-exported model (~91MB fp32, ~23MB int8 quantized)
- `vocab.txt` — WordPiece vocabulary (~30,522 tokens)
- `tokenizer_config.json` — tokenizer settings (max length, special tokens)

**Model inputs (BERT-style):**
- `input_ids`: `int64[batch_size, sequence_length]` — tokenized text
- `attention_mask`: `int64[batch_size, sequence_length]` — 1 for real tokens, 0 for padding
- `token_type_ids`: `int64[batch_size, sequence_length]` — all zeros (single-segment)

**Model output:**
- `last_hidden_state`: `float32[batch_size, sequence_length, 1024]` — per-token embeddings
- Final embedding: mean pool over non-padding tokens → `float32[1024]`

**Query prefix:** mdbr-leaf-mt may use a `prompt_name="query"` prefix for asymmetric retrieval tasks. For our symmetric similarity use case (log line vs taxonomy description), both sides are "documents" — verify during implementation whether omitting the prefix or using a uniform prefix produces better classification results.

---

## Section 1: ONNX Runtime Integration

**What:** Add `onnxruntime-go` as a dependency and build the low-level session management.

### Tasks

1.1 **Add `onnxruntime-go` dependency**
- `go get` the package (likely `github.com/yalue/onnxruntime_go`)
- Verify it builds on Linux aarch64 — this is the host platform
- May need the ONNX Runtime shared library (`libonnxruntime.so`) installed or vendored
- Update `go.mod` / `go.sum`

1.2 **ONNX session lifecycle in `ONNXEmbedder`**
- Update `ONNXEmbedder` struct to hold an ONNX Runtime session, not just a model path
- `New(modelPath)` → load the ONNX model, create an inference session, validate input/output tensor names
- Add a `Close()` method to release the session and runtime resources
- Update `Embedder` interface to include `Close() error` (or use `io.Closer`)
- Fail fast with a clear error if the model file doesn't exist or is corrupt

1.3 **Raw inference wrapper**
- Internal method: `infer(inputIDs, attentionMask, tokenTypeIDs []int64, seqLen int) ([]float32, error)`
- Takes pre-tokenized inputs, runs ONNX inference, returns raw output tensor
- This is the boundary between tokenization and model execution

### Files touched
- `internal/engine/embedder/embedder.go` — struct, interface, lifecycle
- `internal/engine/embedder/onnx.go` — new file, ONNX session wrapper
- `go.mod`, `go.sum`

### Risks
- `onnxruntime-go` may not have pre-built binaries for Linux aarch64 — may need to build ONNX Runtime from source or find an aarch64 release
- The Go bindings API may differ from what's documented; need to read the actual package source

---

## Section 2: WordPiece Tokenizer

**What:** A pure-Go WordPiece tokenizer that loads `vocab.txt` and produces `input_ids` + `attention_mask` compatible with mdbr-leaf-mt.

### Tasks

2.1 **Vocabulary loader**
- Parse `vocab.txt` (one token per line, line number = token ID)
- Build `token → id` map and `id → token` map
- Extract special token IDs: `[CLS]`, `[SEP]`, `[PAD]`, `[UNK]`
- File: `internal/engine/embedder/vocab.go`

2.2 **WordPiece tokenization algorithm**
- Input: raw text string
- Steps:
  1. Lowercase the input (mdbr-leaf-mt is uncased)
  2. Basic tokenization: split on whitespace and punctuation, handle special characters
  3. WordPiece: for each word, greedily match longest prefix in vocab; split remainder with `##` prefix
  4. Prepend `[CLS]`, append `[SEP]`
  5. Truncate to max sequence length (128 tokens is plenty for log lines; mdbr-leaf-mt max is 512)
  6. Generate `attention_mask`: 1 for real tokens, 0 for padding
  7. Generate `token_type_ids`: all zeros
- Output: `input_ids []int64`, `attention_mask []int64`, `token_type_ids []int64`
- File: `internal/engine/embedder/tokenizer.go`

2.3 **Batch tokenization**
- Tokenize multiple texts, pad all sequences to the length of the longest in the batch
- Return 2D slices ready for ONNX tensor construction
- Same file as 2.2

2.4 **Tokenizer tests**
- Test against known tokenization outputs (compare with Python `transformers` tokenizer for a few log lines)
- Edge cases: empty string, very long input (truncation), pure punctuation, unicode, numbers
- File: `internal/engine/embedder/tokenizer_test.go`

### Design decisions
- **Pure Go, no CGo tokenizer bindings.** The vocab is 30K entries and WordPiece is simple enough to implement directly. Avoids a dependency on HuggingFace's Rust tokenizers library.
- **Max sequence length of 128.** Log lines rarely exceed 128 WordPiece tokens. Shorter sequences = faster inference. Configurable if needed later.

### Files touched
- `internal/engine/embedder/vocab.go` — new file
- `internal/engine/embedder/tokenizer.go` — new file
- `internal/engine/embedder/tokenizer_test.go` — new file

---

## Section 3: Embed Implementation

**What:** Wire tokenizer + ONNX inference + mean pooling into the `Embed()` and `EmbedBatch()` methods.

### Tasks

3.1 **Mean pooling**
- Take raw model output `[batch_size, seq_len, 1024]` and attention mask
- For each sequence: sum embeddings of non-padding tokens, divide by count of non-padding tokens
- Return `[]float32` of length 1024 per input
- File: `internal/engine/embedder/pooling.go`

3.2 **`Embed(text string) ([]float32, error)`**
- Tokenize the text (Section 2)
- Run ONNX inference (Section 1)
- Mean pool the output (3.1)
- Return the 1024-dim vector

3.3 **`EmbedBatch(texts []string) ([][]float32, error)`**
- Batch-tokenize all texts with uniform padding
- Single ONNX inference call for the whole batch
- Mean pool each sequence independently
- Return slice of 1024-dim vectors
- Consider a max batch size to bound memory usage (e.g., 64 texts per inference call, loop if more)

3.4 **L2 normalization (optional but recommended)**
- Normalize output vectors to unit length before returning
- Makes cosine similarity equivalent to dot product — slightly faster downstream
- The classifier's `cosineSimilarity` already handles unnormalized vectors, so this is an optimization, not a requirement

3.5 **Embed tests**
- Smoke test: embed a string, verify output is `[]float32` of length 1024
- Determinism: embed the same string twice, verify identical output
- Batch consistency: `Embed(x)` should equal `EmbedBatch([]string{x})[0]`
- Semantic sanity: embed two similar log lines and two dissimilar ones, verify cosine similarity ordering is correct
- File: `internal/engine/embedder/embedder_test.go`

### Files touched
- `internal/engine/embedder/pooling.go` — new file
- `internal/engine/embedder/embedder.go` — update `Embed`, `EmbedBatch`
- `internal/engine/embedder/embedder_test.go` — new file

---

## Section 4: Taxonomy Pre-Embedding

**What:** At startup, embed all taxonomy leaf labels so the classifier has real vectors to compare against.

### Tasks

4.1 **Implement `taxonomy.New()` pre-embedding**
- Walk the taxonomy tree, collect all leaf nodes
- For each leaf, construct the embedding text: `"{Parent.Name}: {Leaf.Desc}"` (e.g., `"ERROR: Network or database connection failure"`)
- Call `embedder.EmbedBatch()` with all label texts
- Store results as `[]model.EmbeddedLabel` with `Path` (e.g., `"ERROR.connection_failure"`) and `Vector`
- This runs once at startup — not in the hot path

4.2 **Update engine initialization**
- `engine.New()` or the startup code in `main.go` / `pipeline` should call `taxonomy.New(roots, embedder)` which now actually embeds
- Verify `taxonomy.Labels()` returns populated `EmbeddedLabel` slices after init

4.3 **Startup logging**
- Log how many labels were embedded and how long it took
- Log the model path and embedding dimension for debugging

### Files touched
- `internal/engine/taxonomy/taxonomy.go` — implement the pre-embedding logic
- `cmd/lumber/main.go` or `internal/pipeline/pipeline.go` — ensure init order is correct

---

## Section 5: Model Download & Makefile

**What:** `make download-model` fetches the ONNX model and vocab files into `models/`.

### Tasks

5.1 **Download script**
- Fetch `model.onnx` (+ `model.onnx_data` if external weights), `vocab.txt`, and `tokenizer_config.json` from HuggingFace (`MongoDB/mdbr-leaf-mt` or `onnx-community/mdbr-leaf-mt-ONNX`)
- Place into `models/` directory
- Verify file integrity (check file size or sha256 if available)
- Use `curl` or `wget` — no Python dependency

5.2 **Update Makefile**
- Replace the `download-model` TODO with actual download commands
- Add the HuggingFace URL for the ONNX model
- Add a check: skip download if files already exist

5.3 **Update `.gitignore`**
- Ensure `models/*.onnx` and `models/vocab.txt` are ignored (don't commit 80MB model files)
- Keep `models/.gitkeep`

5.4 **Update config defaults**
- Verify `LUMBER_MODEL_PATH` default (`models/model.onnx`) aligns with download location
- Add `LUMBER_VOCAB_PATH` config or derive vocab path from model path (e.g., same directory)

### Files touched
- `Makefile`
- `.gitignore`
- `internal/config/config.go` — possibly add vocab path

---

## Section 6: Benchmarking

**What:** Confirm the embedder meets the <10ms per log line performance target.

### Tasks

6.1 **Benchmark test**
- Go benchmark: `BenchmarkEmbed` — single log line embedding latency
- Go benchmark: `BenchmarkEmbedBatch` — batch embedding throughput (batch sizes: 1, 8, 32, 64)
- Report ns/op and allocations
- File: `internal/engine/embedder/bench_test.go`

6.2 **Taxonomy pre-embedding benchmark**
- Measure startup time for embedding all 34 taxonomy labels
- This should be a one-time cost of ~100-300ms

6.3 **Document results**
- Record benchmark numbers in the completion summary (not in this plan)
- Flag if aarch64 performance differs significantly from expectations

### Files touched
- `internal/engine/embedder/bench_test.go` — new file

---

## Implementation Order

```
Section 5 (download model)          — can do immediately, no code deps
    ↓
Section 1 (ONNX runtime)           — needs model files to test
    ↓
Section 2 (tokenizer)              — independent of ONNX, but needed by Section 3
    ↓
Section 3 (embed implementation)   — combines Sections 1 + 2
    ↓
Section 4 (taxonomy pre-embedding) — needs working embedder
    ↓
Section 6 (benchmarking)           — needs everything working
```

Sections 1 and 2 are independent and can be developed in parallel. Section 5 should go first since the model files are needed to test anything.

---

## File Summary

New files:
- `internal/engine/embedder/onnx.go` — ONNX Runtime session wrapper
- `internal/engine/embedder/vocab.go` — vocabulary loader
- `internal/engine/embedder/tokenizer.go` — WordPiece tokenizer
- `internal/engine/embedder/tokenizer_test.go` — tokenizer tests
- `internal/engine/embedder/pooling.go` — mean pooling
- `internal/engine/embedder/embedder_test.go` — embed integration tests
- `internal/engine/embedder/bench_test.go` — benchmarks

Modified files:
- `internal/engine/embedder/embedder.go` — replace stubs with real implementation
- `internal/engine/taxonomy/taxonomy.go` — implement pre-embedding
- `internal/config/config.go` — vocab path config
- `go.mod`, `go.sum` — add `onnxruntime-go`
- `Makefile` — model download
- `.gitignore` — model files

---

## Open Questions

1. **ONNX Runtime on aarch64 Linux:** The `onnxruntime-go` package requires the ONNX Runtime shared library (`libonnxruntime.so`). Pre-built binaries exist for x86_64 and ARM macOS — do you know if you have ONNX Runtime available on this system, or should we plan to build it from source / find an aarch64 Linux release?

2. **Alternative to `onnxruntime-go`:** If ONNX Runtime proves painful on aarch64, we could consider `github.com/nicksrandall/ggml-go` or running inference through a different backend (e.g., calling a tiny embedded Python process, though that violates the "no Python" principle). Do you have a preference, or should we try `onnxruntime-go` first and adapt if it doesn't work?

3. **Tokenizer approach — pure Go vs CGo bindings:** The plan proposes a pure-Go WordPiece tokenizer. An alternative is using HuggingFace's `tokenizers` Rust library via CGo bindings (`github.com/daulet/tokenizers`), which would be faster and guaranteed-compatible but adds a CGo dependency. Preference?

4. **Embedding text for taxonomy labels:** Section 4.1 proposes embedding `"{Parent}: {Description}"` (e.g., `"ERROR: Network or database connection failure"`). An alternative is embedding just the description, or a more elaborate template like `"Log category: {Parent} — {Leaf}: {Description}"`. The choice affects classification quality. Any preference, or should we experiment in Phase 2?

5. **`Embedder` interface — add `Close()`?** The current `Embedder` interface has only `Embed` and `EmbedBatch`. ONNX sessions need cleanup. Should we add `Close() error` to the interface (or embed `io.Closer`), or handle cleanup outside the interface?

6. **mdbr-leaf-mt query prefix:** The model documentation references a `prompt_name="query"` parameter for encoding queries in retrieval tasks. Need to verify during implementation whether our symmetric use case (log ↔ taxonomy description) benefits from a prefix, no prefix, or a uniform prefix on both sides. This can be tested empirically once the embedder is working.
