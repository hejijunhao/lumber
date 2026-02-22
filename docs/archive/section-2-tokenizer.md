# Section 2: WordPiece Tokenizer — Completion Notes

## What Was Done

### 2.1 — Vocabulary loader

Created `internal/engine/embedder/vocab.go` with:

- **`loadVocab(path)`** — parses `vocab.txt` (one token per line, line number = token ID). Builds bidirectional maps (`token→id`, `id→token`). Validates that all required special tokens exist.
- **`vocab` struct** — holds maps and resolved special token IDs: `[PAD]=0`, `[UNK]=100`, `[CLS]=101`, `[SEP]=102`.
- **`lookup(token)`** — returns token ID or `[UNK]` ID for unknown tokens.
- **`contains(token)`** — reports whether a token exists in the vocabulary.
- Vocabulary size: 30,522 tokens (standard BERT WordPiece vocab).

### 2.2 — WordPiece tokenization algorithm

Created `internal/engine/embedder/tokenizer.go` with:

- **`newTokenizer(vocabPath)`** — creates a tokenizer by loading the vocabulary.
- **`tokenize(text)`** — full BERT tokenization pipeline:
  1. **Clean text** — remove control characters, normalize whitespace
  2. **Tokenize Chinese characters** — add spaces around CJK Unified Ideographs
  3. **Lowercase** — `do_lower_case: true` per model config
  4. **Strip accents** — NFD normalization then remove combining diacritical marks (via `golang.org/x/text/unicode/norm`)
  5. **Split on whitespace** — `strings.Fields`
  6. **Split on punctuation** — each punctuation character becomes its own token
  7. **WordPiece** — greedy longest-prefix matching with `##` continuation prefix; single `[UNK]` for tokens that can't be decomposed; 200-rune max per token
  8. **Wrap** — prepend `[CLS]`, append `[SEP]`
  9. **Truncate** — cap at 128 tokens total (126 content tokens + CLS + SEP)
  10. **Pad** — right-pad to `maxSeqLen` (128) with `[PAD]=0`
  11. **Generate masks** — `attention_mask` (1 for real, 0 for padding), `token_type_ids` (all zeros)
- Returns `inputIDs, attentionMask, tokenTypeIDs []int64` — all length 128, ready for ONNX.

### 2.3 — Batch tokenization

- **`tokenizeBatch(texts)`** — tokenizes each text individually, then packs into flat slices padded to the *longest sequence in the batch* (not the global maxSeqLen). This minimizes padding and speeds up inference for short batches.
- Returns a `tokenized` struct with flat `inputIDs`, `attentionMask`, `tokenTypeIDs` slices and `batchSize`/`seqLen` dimensions — ready to pass directly to `onnxSession.infer()`.

### 2.4 — Tokenizer tests

Created `internal/engine/embedder/tokenizer_test.go` with 10 tests:

- **`TestVocabLoad`** — verifies vocab size (30,522) and all special token IDs
- **`TestTokenize`** — 7 sub-tests validated against HuggingFace `BertTokenizer` reference output:
  - `simple` — "hello world" → `[101, 7592, 2088, 102]`
  - `empty string` — "" → `[101, 102]`
  - `log line with punctuation and numbers` — full log line with brackets, dashes, em-dash, parens, equals signs (33 content tokens)
  - `IP address and duration` — "10.0.0.1:5432" splits on dots and colons correctly
  - `accented characters stripped` — "café résumé naïve" → "cafe resume naive"
  - `chinese characters` — CJK chars split individually; 你/好/界 → `[UNK]`, 世 → 1745
  - `mixed punctuation brackets` — "a]b[c" splits correctly on `]` and `[`
- **`TestTokenizeTruncation`** — 200 single-char words → exactly 128 tokens, [CLS] at start, [SEP] at end
- **`TestTokenizeBatch`** — 2 texts, verifies flat packing and both sequences start with [CLS]
- **`TestTokenizeBatchEmpty`** — nil input returns zero batch

All tests produce exact token-for-token matches against HuggingFace's Python `BertTokenizer`.

## Design Decisions

### Pure Go, no CGo tokenizer bindings
WordPiece is simple enough to implement directly (~250 lines). Avoids a dependency on HuggingFace's Rust `tokenizers` library via CGo. The vocab is only 30K entries — lookup is fast via a Go map.

### `golang.org/x/text` for accent stripping
NFD normalization + combining mark removal is the correct way to strip accents (matching BERT's Python implementation). This is the only new dependency added. `x/text` is a standard extended library.

### Max sequence length of 128
Log lines rarely exceed 128 WordPiece tokens. Matches `tokenizer_config.json`'s `max_length: 128`. Shorter sequences = faster inference. The model supports up to 512 but there's no reason to use it.

### Batch padding to longest sequence, not maxSeqLen
`tokenizeBatch` pads to the longest sequence in the batch rather than always padding to 128. For typical log lines (20-40 tokens), this reduces unnecessary computation in the ONNX inference. The trade-off is slightly more logic in the batch function — worth it for the inference speedup.

### Character classification matches BERT exactly
`isPunctuation`, `isWhitespace`, `isControl`, `isChineseChar` all match BERT's Python `BasicTokenizer` definitions, including the ASCII-range punctuation override (BERT treats more chars as punctuation than Go's `unicode.IsPunct`).

## Dependencies Added

- `golang.org/x/text` v0.34.0 — for `unicode/norm.NFD` (accent stripping)
- Go toolchain upgraded from 1.23 to 1.24.0 (automatic, triggered by `go get`)

## Files Changed

- `internal/engine/embedder/vocab.go` — **new**, vocabulary loader
- `internal/engine/embedder/tokenizer.go` — **new**, WordPiece tokenizer + batch tokenization
- `internal/engine/embedder/tokenizer_test.go` — **new**, 10 tests validated against HuggingFace reference
- `go.mod`, `go.sum` — added `golang.org/x/text` v0.34.0, toolchain bump

## Interface for Section 3

The tokenizer produces exactly what `onnxSession.infer()` expects:

- **Single text:** `tokenize(text)` → `inputIDs, attentionMask, tokenTypeIDs []int64` (all length 128)
- **Batch:** `tokenizeBatch(texts)` → `tokenized{inputIDs, attentionMask, tokenTypeIDs, batchSize, seqLen}` with flat slices ready for `infer()`

Section 3 wiring is straightforward:
1. `tokenize(text)` → `infer(ids, mask, types, 1, 128)` → mean pool → project → 1024-dim vector
2. `tokenizeBatch(texts)` → `infer(ids, mask, types, batchSize, seqLen)` → mean pool each → project each → batch of 1024-dim vectors
