# Section 6 — Post-Review Fixes

Fixes for issues 1–5 from `docs/executing/embedding-engine-review.md`. Issue 6 (L2 normalization) deferred as recommended.

---

## Fix 1: `ProcessBatch` batching

**Problem:** `ProcessBatch` looped over raws calling `Process` individually — N separate ONNX inference calls instead of one batched call, defeating the purpose of `EmbedBatch`.

**File:** `internal/engine/engine.go`

**Change:** Extracted all raw texts into a slice, calls `EmbedBatch` once for the full batch, then classifies/compacts per-event using the returned vectors. Added early return for empty input.

---

## Fix 2: `math.Sqrt` replacement

**Problem:** Custom Newton's method with 64 iterations instead of `math.Sqrt`, which compiles to a single CPU instruction. Slower and harder to read for no benefit.

**File:** `internal/engine/classifier/classifier.go`

**Change:** Replaced custom `sqrt` function with `math.Sqrt`. Removed the 10-line custom implementation entirely. Added `"math"` import.

---

## Fix 3: Leaf-level severity

**Problem:** `inferSeverity` only mapped top-level types (`ERROR`→error, `SECURITY`→warning, default→info). This produced misleading severity for error-like leaves under non-error parents (e.g. `DEPLOY.build_failed`→"info", `SCHEDULED.cron_failed`→"info").

**Files:**
- `internal/model/taxonomy.go` — added `Severity` field to `TaxonomyNode` and `EmbeddedLabel`
- `internal/engine/taxonomy/default.go` — set severity on every leaf node
- `internal/engine/taxonomy/taxonomy.go` — propagated severity from node to embedded label
- `internal/engine/taxonomy/taxonomy_test.go` — added severity values to test nodes and assertion
- `internal/engine/engine.go` — replaced `inferSeverity(eventType)` with `result.Label.Severity`, removed the `inferSeverity` function

**Severity assignments:**
| Leaf | Severity |
|------|----------|
| All ERROR children | error (except `validation_error` → warning) |
| `DEPLOY.build_failed`, `DEPLOY.deploy_failed` | error |
| `SCHEDULED.cron_failed` | error |
| `SYSTEM.resource_limit` | warning |
| `SECURITY.login_failure`, `rate_limited`, `suspicious_activity` | warning |
| `APPLICATION.warning` | warning |
| `DATA.cache_hit`, `APPLICATION.debug` | debug |
| Everything else | info |

---

## Fix 4: Single-text dynamic padding

**Problem:** `Embed()` always ran inference on 128 positions (maxSeqLen) even for short inputs. The batch path got dynamic padding via `tokenizeBatch`, but the single-text path didn't.

**File:** `internal/engine/embedder/embedder.go`

**Change:** `Embed()` now routes through `tokenizeBatch` with a 1-element slice instead of calling `tokenize` directly. This gives the same dynamic-padding-to-longest behavior as `EmbedBatch`. For a 10-token log line, inference now runs on ~12 positions instead of 128.

---

## Fix 5: Makefile `LD_LIBRARY_PATH`

**Problem:** `make test` ran `go test ./...` without `LD_LIBRARY_PATH=models`. Tests worked when run from the repo root (because `newONNXSession` derives the library path from `filepath.Dir(modelPath)`), but would break from other directories.

**File:** `Makefile`

**Change:** Prefixed the test command with `LD_LIBRARY_PATH=$(MODEL_DIR)`.

---

## Deferred

**Issue 6: L2 normalization of final embeddings.** Not a bug — cosine similarity in the classifier handles unnormalized vectors correctly. Will matter if embeddings are used outside the classifier (similarity thresholds, clustering for adaptive taxonomy). Deferred until adaptive taxonomy work.

---

## Verification

All tests pass after all five fixes (`make test`).
