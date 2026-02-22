# Section 4: Taxonomy Pre-Embedding — Completion Notes

## What Was Done

### 4.1 — Implement `taxonomy.New()` pre-embedding

Updated `internal/engine/taxonomy/taxonomy.go`:

- **Leaf collection** — walks the `roots` slice, iterates each root's `Children`, and builds parallel slices of paths (`"ERROR.connection_failure"`) and embedding texts (`"ERROR: Connection failed"`).
- **`emb.EmbedBatch(texts)`** — single batch call embeds all leaf labels at once. With the default taxonomy (34 leaves), this is one inference call.
- **Label construction** — zips paths and vectors into `[]model.EmbeddedLabel` and stores on the `Taxonomy` struct.
- **Edge cases** — empty roots or roots with no children short-circuit before calling the embedder, returning a valid `Taxonomy` with zero labels.

### 4.2 — Startup logging

Updated `cmd/lumber/main.go`:

- After embedder init: logs model path and embedding dimension (`dim=1024`).
- After taxonomy init: logs label count and wall-clock duration (`pre-embedded 34 labels in 142ms`).
- Both lines go to stderr, matching the existing `lumber:` prefix convention.

### 4.3 — Tests

Created `internal/engine/taxonomy/taxonomy_test.go` with 4 tests:

- **`TestNewPreEmbeds`** — 2 roots with 3 total leaves; verifies correct paths (`ERROR.timeout`, `ERROR.connection_failure`, `SYSTEM.startup`), correct vector dimension, and non-zero values.
- **`TestNewEmptyRoots`** — nil roots returns 0 labels without calling the embedder.
- **`TestNewNoLeaves`** — root-only nodes (no children) returns 0 labels without calling the embedder.
- **`TestNewEmbedError`** — embedder failure propagates as a wrapped error.

All 4 tests pass.

## Design Decisions

### Embedding text format: `"{Parent}: {Leaf.Desc}"`

The embedding text for each leaf concatenates the parent category name with the leaf description — e.g., `"ERROR: Network or database connection failure"`. This gives the embedding model both the high-level category context and the semantic description. Alternatives considered:
- Description only (`"Network or database connection failure"`) — loses category context, may conflate similar descriptions across categories.
- Full path (`"ERROR.connection_failure: Network or database connection failure"`) — the dotted path is a code identifier, not natural language; unlikely to help the embedding model.

### Single `EmbedBatch` call

All labels are embedded in one batch call rather than individual `Embed` calls. With 34 labels this is a single ONNX inference pass, keeping startup fast (~100-300ms expected).

### No taxonomy-level caching

Pre-embedding runs once at startup. The taxonomy is fixed for the lifetime of the process (adaptive taxonomy is a future feature), so there's no need to cache or invalidate embeddings.

## Dependencies Added

None.

## Files Changed

- `internal/engine/taxonomy/taxonomy.go` — replaced stub with leaf collection + `EmbedBatch` + label construction
- `internal/engine/taxonomy/taxonomy_test.go` — **new**, 4 unit tests with mock embedder
- `cmd/lumber/main.go` — added `time` import, startup logging for embedder and taxonomy init

## Interface for Section 6

The full pipeline is now functional end-to-end:

1. **Embedder** loads ONNX model, vocab, and projection weights
2. **Taxonomy** pre-embeds all 34 leaf labels via `EmbedBatch` at startup
3. **Engine** embeds incoming log lines and classifies against pre-embedded taxonomy labels via cosine similarity
4. **Classifier** returns the best-matching label path and confidence score

Section 6 (benchmarking) can now measure real inference latency and taxonomy pre-embedding time.
