# Section 5: Model Download & Makefile — Completion Notes

## What Was Done

### 5.1 / 5.2 — Makefile `download-model` target

Replaced the TODO stub with working download commands that fetch from the `onnx-community/mdbr-leaf-mt-ONNX` HuggingFace repo.

**Model variant chosen: `model_quantized` (int8)**
- `model.onnx` — 216KB (model graph definition)
- `model.onnx_data` — 22MB (int8-quantized weights)
- Total: ~23MB vs ~92MB for fp32

Why int8 over fp32: log classification doesn't need fp32 precision. The quantized model is 4x smaller, loads faster, and runs faster on CPU. If classification quality is insufficient, switching to fp32 is a one-line URL change in the Makefile.

**Tokenizer files downloaded:**
- `vocab.txt` — 227KB, 30,522 WordPiece tokens (standard BERT vocab)
- `tokenizer_config.json` — confirms: `BertTokenizer`, `do_lower_case: true`, `max_length: 128`, `model_max_length: 512`

**Idempotency:** The target checks for existing files and skips the download if all three key files (`model.onnx`, `model.onnx_data`, `vocab.txt`) are present.

### 5.3 — `.gitignore` updates

Added patterns for `*.onnx_data`, `vocab.txt`, and `tokenizer_config.json` under the models directory. The `.gitkeep` file remains tracked.

### 5.4 — Config: `VocabPath`

Added `VocabPath` to `EngineConfig` with env var `LUMBER_VOCAB_PATH` (default: `models/vocab.txt`). This is needed by the tokenizer in Section 2.

## Key Facts for Subsequent Sections

From `tokenizer_config.json`:
- Tokenizer class: `BertTokenizer` (WordPiece)
- Uncased: `do_lower_case: true`
- Max sequence length: 128 (with model max of 512)
- Special token IDs: `[PAD]=0`, `[UNK]=100`, `[CLS]=101`, `[SEP]=102`, `[MASK]=103`
- Padding side: right

Model output: 1024-dimensional embeddings (confirmed by plan; to be verified empirically in Section 1 via ONNX model inspection).

## Files Changed

- `Makefile` — real download commands
- `.gitignore` — model file patterns
- `internal/config/config.go` — added `VocabPath` field and env default
