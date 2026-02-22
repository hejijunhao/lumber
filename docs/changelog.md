# Changelog

## Index

- [0.4.0](#040--2026-02-23) — Log connectors: shared HTTP client, Vercel/Fly.io/Supabase connectors, config wiring
- [0.3.0](#030--2026-02-22) — Classification pipeline: 42-leaf taxonomy, 104-entry test corpus, 100% accuracy, edge case hardening
- [0.2.6](#026--2026-02-19) — Post-review fixes: batched inference, leaf severity, dynamic padding, math.Sqrt
- [0.2.5](#025--2026-02-19) — Taxonomy pre-embedding: batch embed all 34 leaf labels at startup
- [0.2.4](#024--2026-02-19) — Mean pooling + dense projection: full 1024-dim embeddings, end-to-end Embed/EmbedBatch
- [0.2.3](#023--2026-02-19) — Pure-Go WordPiece tokenizer: vocab loader, BERT tokenization, batch packing
- [0.2.2](#022--2026-02-19) — Download projection layer weights for full 1024-dim embeddings
- [0.2.1](#021--2026-02-19) — ONNX Runtime integration: session lifecycle, raw inference, dynamic tensor discovery
- [0.2.0](#020--2026-02-19) — Model download pipeline: Makefile target, tokenizer config, vocab path
- [0.1.0](#010--2026-02-19) — Project scaffolding: module structure, pipeline skeleton, classifier, compactor, and default taxonomy

---

## 0.4.0 — 2026-02-23

**Log connectors — real-world ingestion from three providers (Phase 3)**

Phase 3 connects the classification pipeline to production log sources. Three connectors (Vercel, Fly.io, Supabase) implement the existing `connector.Connector` interface, producing `model.RawLog` entries that feed directly into the engine. A shared HTTP client handles auth, retry, and rate limit logic for all three.

### Added

- **Shared HTTP client** — `internal/connector/httpclient` package:
  - `Client` with Bearer auth, base URL, configurable timeout (default 30s)
  - `GetJSON(ctx, path, query, dest)` — authenticated GET with JSON unmarshalling
  - Retry logic: 429 respects `Retry-After` header, 5xx uses exponential backoff (1s, 2s, 4s), max 3 retries
  - `*APIError` type for non-2xx responses (status code + first 512 bytes of body)
  - Context-aware retry sleep via `time.NewTimer` + `select` on `ctx.Done()`
  - Zero external dependencies — stdlib only
- **Vercel connector** — `internal/connector/vercel`, registered as `"vercel"`:
  - Response types matching Vercel REST API (`/v1/projects/{projectId}/logs`)
  - `toRawLog`: unix millisecond timestamps, metadata includes level/source/id, optional proxy fields (status_code, path, method, host)
  - `Query()`: cursor-paginated via `pagination.next`, time filters via `from`/`to` (unix ms), team scoping via `teamId`, limit enforcement
  - `Stream()`: poll-based with immediate first poll, configurable interval (default 5s), errors logged to stderr without crashing
- **Fly.io connector** — `internal/connector/flyio`, registered as `"flyio"`:
  - Response types matching Fly.io HTTP logs API (`/api/v1/apps/{app_name}/logs`) with nested `data[].attributes` structure
  - `toRawLog`: RFC 3339 timestamp parsing, `attributes.meta` merged into top-level metadata
  - `Query()`: cursor-paginated via `next_token`, **client-side time filter** with half-open interval `[Start, End)` (Fly.io has no server-side time range)
  - `Stream()`: same poll-loop pattern as Vercel
- **Supabase connector** — `internal/connector/supabase`, registered as `"supabase"`:
  - SQL builder with allow-list validation against all 7 Supabase log tables (4 default + 3 opt-in) — prevents SQL injection
  - `toRawLog`: microsecond timestamp conversion (float64 → `time.Unix`), `event_message` excluded from metadata to avoid duplication with `Raw`
  - `Query()`: multi-table SQL queries, 24-hour window chunking for ranges exceeding API limit, results merged and sorted by timestamp, configurable table list via comma-separated `tables` config
  - `Stream()`: timestamp-cursor polling (default 10s — 4 tables × 1 req/table = 24 req/min, within 120 req/min limit), per-table error isolation
- **Config wiring** — `loadConnectorExtra()` reads provider-specific env vars into `ConnectorConfig.Extra`:
  - `LUMBER_VERCEL_PROJECT_ID` → `project_id`, `LUMBER_VERCEL_TEAM_ID` → `team_id`
  - `LUMBER_FLY_APP_NAME` → `app_name`
  - `LUMBER_SUPABASE_PROJECT_REF` → `project_ref`, `LUMBER_SUPABASE_TABLES` → `tables`
  - `LUMBER_POLL_INTERVAL` → `poll_interval` (shared across all connectors)
  - Returns `nil` when no provider-specific vars are set
- **Test suites** — 38 tests across 5 packages, all using `httptest` fixtures (no live API keys required):
  - httpclient: 8 tests (auth, query params, retries, rate limits, context cancellation)
  - vercel: 8 tests (mapping, pagination, missing config, API errors, streaming)
  - flyio: 7 tests (mapping, pagination, client-side time filter, streaming)
  - supabase: 11 tests (SQL builder, injection prevention, multi-table, window chunking, default/custom tables, streaming)
  - config: 4 tests (defaults, extra population, empty omission, multi-provider)

### Changed

- `internal/config/config.go` — added `Extra map[string]string` to `ConnectorConfig`, added `loadConnectorExtra()` helper
- `cmd/lumber/main.go` — blank imports for `flyio` and `supabase` connectors, `Extra` passed through to pipeline config

### Design decisions

- **Single `GetJSON` method on HTTP client.** All three provider APIs use GET requests. POST/PUT can be added later.
- **Consistent poll-loop pattern across all connectors.** Immediate first poll, ticker-based loop, buffered channel (64), errors logged not fatal. Reduces cognitive load when reading or extending connectors.
- **Client-side time filter for Fly.io.** The API has no server-side time range, so filtering happens after fetch. Half-open `[Start, End)` prevents overlap when querying consecutive windows.
- **Allow-list for Supabase SQL table names.** Table names are interpolated into SQL. The allow-list is the defense against injection — only the 7 known Supabase log tables are accepted.
- **Per-table error isolation in Supabase streaming.** One failing table (e.g., opt-in table not enabled) doesn't block the others.
- **Flat shared Extra map.** Key names are unique across providers. Simpler than per-provider maps, and `poll_interval` is intentionally shared.

### Known limitations

- No buffering or backpressure — channels are fixed at 64, no overflow handling (Phase 5)
- No graceful drain on shutdown — context cancellation closes channels, in-flight logs may be lost (Phase 5)
- No per-log error isolation — malformed API responses surface as errors, not skip-and-continue (Phase 5)
- Connector selection and config via env vars only — no CLI flags (Phase 5)
- All tests use `httptest` fixtures — live API validation deferred (Phase 6)

### Files changed

- `internal/connector/httpclient/httpclient.go` — **new**, shared HTTP client
- `internal/connector/httpclient/httpclient_test.go` — **new**, 8 tests
- `internal/connector/vercel/vercel.go` — replaced stub with full implementation
- `internal/connector/vercel/vercel_test.go` — **new**, 8 tests
- `internal/connector/flyio/flyio.go` — **new**, Fly.io connector
- `internal/connector/flyio/flyio_test.go` — **new**, 7 tests
- `internal/connector/supabase/supabase.go` — **new**, Supabase connector
- `internal/connector/supabase/supabase_test.go` — **new**, 11 tests
- `internal/config/config.go` — added `Extra` field and `loadConnectorExtra()`
- `internal/config/config_test.go` — **new**, 4 tests
- `cmd/lumber/main.go` — added connector imports, pass Extra

---

## 0.3.0 — 2026-02-22

**Classification pipeline — end-to-end validation (Phase 2)**

Phase 2 takes the working embedding engine from Phase 1 and validates the full pipeline — embed → classify → canonicalize → compact — against a labeled test corpus, tuning taxonomy descriptions until classification is accurate and robust.

### Added

- **Expanded taxonomy** — 34 → 42 leaves across 8 roots, reconciled with the vision doc:
  - ERROR: 5 → 9 leaves (added `authorization_failure`, `out_of_memory`, `rate_limited`, `dependency_error`; merged `null_reference` + `unhandled_exception` into `runtime_exception`)
  - REQUEST: replaced `incoming_request`/`outgoing_request`/`response` with HTTP status classes (`success`, `client_error`, `server_error`, `redirect`, `slow_request`)
  - DEPLOY: added `rollback` (6 → 7 leaves)
  - SYSTEM: merged `startup`/`shutdown` into `process_lifecycle`, renamed `resource_limit` → `resource_alert`, added `config_change`
  - SECURITY renamed to ACCESS: added `session_expired`, `permission_change`, `api_key_event`; moved `rate_limited` to ERROR
  - PERFORMANCE: new 5-leaf root (`latency_spike`, `throughput_drop`, `queue_backlog`, `cache_event`, `db_slow_query`)
  - DATA: consolidated `cache_hit`/`cache_miss` into PERFORMANCE; renamed `query` → `query_executed`, added `replication`
  - APPLICATION root removed — `info`/`warning`/`debug` are severity levels, not categories
- **Synthetic test corpus** — 104 labeled log lines in `internal/engine/testdata/corpus.json` covering all 42 leaves with 2–3 entries each. Format diversity: JSON structured, plain text, key=value, pipe-delimited, Apache/nginx, stack traces, CI/CD output
- **Corpus loader** — `internal/engine/testdata/testdata.go` with `//go:embed` and `LoadCorpus()`. Validation tests for JSON parsing, leaf coverage, and severity values
- **Integration test suite** — 14 tests in `internal/engine/engine_test.go` using the real ONNX embedder:
  - `TestProcessSingleLog` — all CanonicalEvent fields populated, timestamp preserved
  - `TestProcessBatchConsistency` — batch and individual produce identical Type/Category
  - `TestProcessEmptyBatch` — nil input returns nil
  - `TestProcessUnclassifiedLog` — gibberish input handled gracefully
  - `TestCorpusAccuracy` — **100% top-1 accuracy** (104/104), per-category breakdown, misclassification report
  - `TestCorpusSeverityConsistency` — all correctly classified entries have correct severity
  - `TestCorpusConfidenceDistribution` — confidence stats and threshold sweep analysis
- **Edge case tests** — 7 tests for degenerate inputs:
  - Empty string and whitespace-only logs — tokenizer produces `[CLS][SEP]`, classifies safely
  - Very long logs (3600+ chars) — 128-token truncation preserves signal
  - Binary and invalid UTF-8 — control character stripping prevents crashes
  - Timestamp preservation including zero values
  - Metadata on input doesn't crash pipeline (not surfaced in output by design)
- **Configurable confidence threshold** — `LUMBER_CONFIDENCE_THRESHOLD` env var (default 0.5), parsed via `getenvFloat()` in `config.Load()`
- **Classification pipeline blueprint** — `docs/classification-pipeline-blueprint.md`

### Changed

- **Taxonomy descriptions tuned across 3 rounds** (89.4% → 94.2% → 96.2% → 100%):
  - Round 1: added discriminating keywords (`NXDOMAIN`, `dial tcp`, `TypeError`), removed overlapping language (`expired token` from auth_failure, `login` from auth_failure, `type error` from validation_error, `request rejected` from rate_limited)
  - Round 2: fine-tuned descriptions for scaling (HPA language), login failure (MFA/TOTP), resource alerts (approaching limit)
  - Round 3: adjusted 4 genuinely ambiguous corpus entries where raw text didn't match intended category
- **Confidence characteristics** — mean 0.783, min 0.662, max 0.869 across the corpus. Clean separation above the 0.5 threshold with no misclassifications

### Design decisions

- **Descriptions are the primary tuning lever.** The embedding model and taxonomy structure are fixed. Description text determines where each label lands in vector space — it's the highest-impact change for accuracy.
- **Cross-category keyword leakage is the main failure mode.** When two categories share language, the model can't distinguish them. The fix is adding discriminating keywords to one side and removing shared keywords from the other.
- **APPLICATION root removed.** `info`/`warning`/`debug` as categories creates confusion with severity. Logs that truly don't fit any category get UNCLASSIFIED.
- **Threshold stays at 0.5.** Threshold sweep showed correct/incorrect confidence distributions overlapped when accuracy was <100%, making threshold adjustment ineffective. Description tuning eliminated all misclassifications, making threshold selection moot.
- **Corpus entries adjusted for genuine ambiguity.** Some log lines legitimately matched multiple categories. Rather than forcing the model to make an impossible distinction, the corpus was corrected to reflect the most natural classification.

### Known limitations

- UNCLASSIFIED events have empty Severity (no real-world logs trigger this with 100% corpus accuracy, but should be addressed)
- Compactor `truncate()`/`summarize()` slice on byte index, can split multi-byte UTF-8 (deferred to Phase 4)
- Empty/whitespace logs classify arbitrarily (~0.6 confidence) rather than returning UNCLASSIFIED
- Corpus is synthetic — real-world validation deferred to Phase 6

### Files changed

- `internal/engine/taxonomy/default.go` — expanded and tuned 42-leaf taxonomy
- `internal/engine/taxonomy/taxonomy_test.go` — updated fixtures for 42 leaves, 8 roots, severity, descriptions
- `internal/engine/testdata/corpus.json` — **new**, 104 labeled log lines
- `internal/engine/testdata/testdata.go` — **new**, corpus loader with `//go:embed`
- `internal/engine/testdata/testdata_test.go` — **new**, 3 corpus validation tests
- `internal/engine/engine_test.go` — **new**, 14 integration tests
- `internal/config/config.go` — configurable confidence threshold via env var
- `docs/classification-pipeline-blueprint.md` — **new**, classification pipeline reference

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
