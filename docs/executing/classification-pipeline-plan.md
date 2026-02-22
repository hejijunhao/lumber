# Phase 2: Classification Pipeline — Implementation Plan

## Goal

End-to-end classification that produces correct, meaningful results: raw log text in → canonical event out, validated against a comprehensive test corpus.

Phase 1 delivered the embedding engine (tokenizer, ONNX inference, mean pooling, projection, taxonomy pre-embedding). Phase 2 takes the working embedder and validates the full pipeline — embed → classify → canonicalize → compact — against real-world log patterns, tuning the taxonomy and confidence threshold until classification accuracy exceeds 80% at top-1.

**Success criteria:**
- Default taxonomy aligned with the vision doc's full label set (~45 leaves across 6–8 root categories)
- Synthetic test corpus of ~100+ log lines covering every taxonomy leaf
- Full engine pipeline unit-tested: `Process(RawLog) → CanonicalEvent` with correct type, category, severity, confidence
- Classification accuracy >80% correct at top-1 against the test corpus
- Confidence threshold tuned so unclassified logs are genuinely ambiguous, not misses
- `ProcessBatch` tested for consistency with per-item `Process`

---

## Current State (What Phase 1 Delivered)

**Working:**
- `Embed(text)` → 1024-dim vector via ONNX Runtime (tokenize → infer → mean pool → project)
- `EmbedBatch(texts)` → batched inference, single ONNX call
- `taxonomy.New(roots, embedder)` → pre-embeds all leaf labels at startup (~34 labels in ~100–300ms)
- `classifier.Classify(vector, labels)` → cosine similarity, threshold-gated, returns best match
- `engine.Process(RawLog)` → full pipeline: embed → classify → compact → `CanonicalEvent`
- `engine.ProcessBatch([]RawLog)` → batched version
- Per-leaf severity on `EmbeddedLabel` (no more `inferSeverity`)
- Compactor with 3 verbosity levels (minimal/standard/full)

**Not yet validated:**
- Whether the 34-leaf taxonomy is sufficient or produces misclassifications for common log patterns
- Whether the confidence threshold (currently hardcoded 0.5) is well-calibrated
- Whether the embedding text format (`"{Parent}: {Leaf.Desc}"`) produces good semantic separation between categories
- End-to-end correctness of the full pipeline with realistic inputs
- Classification accuracy on diverse log formats

---

## Taxonomy Gap Analysis

The current `default.go` taxonomy (34 leaves, 8 roots) diverges from the vision doc's taxonomy (~38 leaves, 6 roots). Phase 2 must reconcile these.

### Current taxonomy (default.go)

```
ERROR (5):      runtime_exception, connection_failure, timeout, auth_failure, validation_error
REQUEST (3):    incoming_request, outgoing_request, response
DEPLOY (6):     build_started, build_succeeded, build_failed, deploy_started, deploy_succeeded, deploy_failed
SYSTEM (5):     startup, shutdown, health_check, resource_limit, scaling
SECURITY (4):   login_success, login_failure, rate_limited, suspicious_activity
DATA (4):       query, migration, cache_hit, cache_miss
SCHEDULED (3):  cron_started, cron_completed, cron_failed
APPLICATION (3): info, warning, debug
```

### Vision doc taxonomy

```
ERROR (11):     connection_failure, authentication_failure, authorization_failure, timeout,
                null_reference, unhandled_exception, validation_error, rate_limited,
                out_of_memory, disk_full, dependency_error
REQUEST (5):    success, client_error, server_error, redirect, slow_request
DEPLOY (7):     build_started, build_succeeded, build_failed, deploy_started,
                deploy_succeeded, deploy_failed, rollback
SYSTEM (5):     health_check, scaling_event, resource_alert, process_lifecycle, config_change
ACCESS (5):     login_success, login_failure, session_expired, permission_change, api_key_event
PERFORMANCE (5): latency_spike, throughput_drop, queue_backlog, cache_miss_spike, db_slow_query
```

### Reconciliation strategy

The vision doc taxonomy is more granular for errors (11 leaves vs 5) and more focused for requests (HTTP status classes vs incoming/outgoing). The current taxonomy adds useful categories not in the vision doc (DATA, SCHEDULED, APPLICATION). The merged taxonomy should keep what's useful from both.

Detailed mapping decisions deferred to Section 1 implementation, where each leaf's description text (which determines embedding quality) can be written carefully.

---

## Section 1: Taxonomy Alignment

**What:** Expand the default taxonomy to cover the vision doc's full label set while retaining useful additions from the current implementation. Rewrite leaf descriptions for maximum embedding quality.

### Tasks

1.1 **Design the merged taxonomy**

Target structure (~45 leaves across 8 roots):

```
ERROR (9):
  connection_failure    — TCP/connection refused/timeout to databases and services
  auth_failure          — authentication failure, invalid token, expired session
  authorization_failure — permission denied, forbidden, insufficient scope
  timeout               — request or operation timeout exceeding deadline
  runtime_exception     — unhandled exception, panic, segfault, null reference
  validation_error      — input validation failure, schema mismatch, type error
  out_of_memory         — OOM kill, heap exhaustion, allocation failure
  rate_limited          — HTTP 429, throttling, quota exceeded
  dependency_error      — upstream or downstream service failure, circuit breaker open

REQUEST (5):
  success               — HTTP 2xx response, successful API call
  client_error          — HTTP 4xx response (excluding auth errors)
  server_error          — HTTP 5xx response, internal server error
  redirect              — HTTP 3xx response, URL redirect
  slow_request          — request exceeding latency threshold, slow API call

DEPLOY (7):
  build_started         — build process initiated
  build_succeeded       — build completed successfully
  build_failed          — build failed with errors
  deploy_started        — deployment initiated
  deploy_succeeded      — deployment completed successfully
  deploy_failed         — deployment failed
  rollback              — deployment rollback triggered

SYSTEM (5):
  health_check          — liveness or readiness probe, health endpoint
  scaling_event         — autoscale up or down, instance count change
  resource_alert        — CPU, memory, or disk threshold breach
  process_lifecycle     — service start, stop, restart, crash, signal received
  config_change         — environment variable update, feature flag toggle

ACCESS (5):
  login_success         — successful user authentication or login
  login_failure         — failed login attempt, bad credentials
  session_expired       — session timeout, token expiration
  permission_change     — role or permission grant, revocation, modification
  api_key_event         — API key created, rotated, or revoked

PERFORMANCE (5):
  latency_spike         — p50/p95/p99 latency degradation
  throughput_drop       — request rate decrease, traffic drop
  queue_backlog         — job or message queue growth, consumer lag
  cache_event           — cache hit, miss, eviction, hit ratio change
  db_slow_query         — database query exceeding execution time threshold

DATA (3):
  query_executed        — database query execution log
  migration             — database schema migration event
  replication           — data replication, sync, or backup event

SCHEDULED (3):
  cron_started          — scheduled job or cron task started
  cron_completed        — scheduled job completed successfully
  cron_failed           — scheduled job failed
```

Total: 42 leaves across 8 roots.

Key changes from current taxonomy:
- ERROR gains `authorization_failure`, `out_of_memory`, `rate_limited` (moved from SECURITY), `dependency_error`
- ERROR merges `runtime_exception` with vision doc's `null_reference` + `unhandled_exception` (too similar to separate with embedding)
- REQUEST replaces incoming/outgoing/response with HTTP status classes (success, client_error, server_error, redirect, slow_request)
- SECURITY renamed to ACCESS (matches vision doc, less ambiguous)
- PERFORMANCE added from vision doc
- DATA slimmed (cache_hit/cache_miss → cache_event under PERFORMANCE; query renamed to query_executed for clarity)
- APPLICATION removed — `info`/`warning`/`debug` are severity levels, not categories. Logs that are "just informational" should classify into a specific category with low confidence or into the nearest semantic match

1.2 **Update `default.go`**
- Replace the current taxonomy tree with the merged version above
- Write high-quality descriptions for every leaf — these are the texts that get embedded, so they must be semantically rich and unambiguous
- Set severity on every leaf following the pattern established in Phase 1

1.3 **Update `taxonomy_test.go`**
- Update test fixtures to match the new taxonomy
- Verify leaf count (42), root count (8), severity assignments

### Design decisions

- **Removing APPLICATION root:** The vision doc doesn't have it, and `info`/`warning`/`debug` as categories creates confusion with severity. A log saying "User signed up" isn't an "info category" — it's an ACCESS event. If a log truly doesn't fit any category, the classifier should return UNCLASSIFIED.
- **Merging null_reference + unhandled_exception into runtime_exception:** The embedding model can't reliably distinguish "NullPointerException" from "unhandled panic" — they're semantically too similar. One leaf with a rich description covers both.
- **cache_hit/cache_miss → cache_event under PERFORMANCE:** Individual cache hits are noise; the signal is cache behavior patterns. This aligns with the vision doc's `cache_miss_spike`.
- **disk_full removed:** Merged into `resource_alert` under SYSTEM. Embedding can't reliably distinguish "disk full" from "memory full" from "CPU limit" — they're all resource exhaustion.

### Files changed

- `internal/engine/taxonomy/default.go` — rewrite taxonomy tree
- `internal/engine/taxonomy/taxonomy_test.go` — update fixtures

---

## Section 2: Synthetic Test Corpus

**What:** Build a corpus of ~100+ realistic log lines, each labeled with its expected taxonomy classification, to validate and tune the classifier.

### Tasks

2.1 **Design the corpus structure**

Create `internal/engine/testdata/corpus.json` with the following schema:

```json
[
  {
    "raw": "ERROR [2026-02-19 12:00:00] UserService — connection refused (host=db-primary, port=5432)",
    "expected_type": "ERROR",
    "expected_category": "connection_failure",
    "expected_severity": "error",
    "description": "PostgreSQL connection refused"
  }
]
```

Each entry is a realistic log line with the expected classification. The `description` field is for human readability only (not used in tests).

2.2 **Write corpus entries**

Target: 2–3 log lines per taxonomy leaf (42 leaves × ~2.5 = ~105 entries). Cover:

- **Format diversity:** JSON logs, plain text, structured key=value, mixed formats
- **Provider styles:** Vercel-style, AWS-style, generic application logs
- **Edge cases per category:**
  - ERROR.connection_failure: TCP, DNS, TLS, database, Redis, HTTP connection errors
  - REQUEST.success: various HTTP 200 log formats, different web servers
  - DEPLOY.build_failed: CI/CD failure messages from different build systems
  - etc.
- **Ambiguous cases:** Logs that could plausibly match multiple categories — these test the classifier's discrimination ability and help calibrate the threshold
- **Severity validation:** Each entry's expected severity must match the taxonomy leaf's assigned severity

2.3 **Corpus loader**

Create `internal/engine/testdata/loader.go` (or use `embed` directive in the test file):

```go
//go:embed corpus.json
var corpusJSON []byte

type CorpusEntry struct {
    Raw              string `json:"raw"`
    ExpectedType     string `json:"expected_type"`
    ExpectedCategory string `json:"expected_category"`
    ExpectedSeverity string `json:"expected_severity"`
    Description      string `json:"description"`
}

func LoadCorpus() ([]CorpusEntry, error) { ... }
```

### Design decisions

- **JSON corpus, not Go structs:** Easier to review, edit, and expand. Can be shared with non-Go tools for analysis.
- **2–3 entries per leaf:** Enough to validate discrimination without creating a huge corpus. Can expand later for specific weak categories.
- **Format diversity over category coverage:** Two very different log formats for the same category tests more than five similar formats would.

### Files changed

- `internal/engine/testdata/corpus.json` — **new**, ~105 log entries
- `internal/engine/testdata/testdata.go` — **new**, corpus loader (or embed in test file)

---

## Section 3: Engine Pipeline Tests

**What:** Unit tests for the full `engine.Process()` and `engine.ProcessBatch()` pipeline using the real embedder and taxonomy.

### Tasks

3.1 **Full pipeline integration test**

File: `internal/engine/engine_test.go`

This test requires the ONNX model files (so it will be skipped in CI without models). Use a test helper:

```go
func skipWithoutModel(t *testing.T) {
    t.Helper()
    if _, err := os.Stat("../../models/model_quantized.onnx"); os.IsNotExist(err) {
        t.Skip("ONNX model not available, skipping integration test")
    }
}
```

Tests:
- **`TestProcessSingleLog`** — Process a single RawLog, verify all CanonicalEvent fields are populated (Type, Category, Severity non-empty; Confidence > 0; Timestamp preserved; Summary non-empty; Raw preserved at Standard verbosity)
- **`TestProcessBatchConsistency`** — Process N logs individually via `Process` and together via `ProcessBatch`, verify identical results (same Type, Category, Confidence for each)
- **`TestProcessEmptyBatch`** — `ProcessBatch(nil)` returns `nil, nil`
- **`TestProcessUnclassifiedLog`** — Feed a log that's genuinely ambiguous (random gibberish), verify it returns UNCLASSIFIED when below threshold

3.2 **Corpus-based accuracy test**

File: `internal/engine/engine_test.go`

```go
func TestCorpusAccuracy(t *testing.T) {
    // Load corpus, init real engine, classify every entry
    // Track: correct (top-1 match), incorrect, unclassified
    // Report accuracy percentage
    // Fail if accuracy < 80%
}
```

This is the key validation test. It:
- Loads the full corpus from `testdata/corpus.json`
- Initializes a real engine (embedder, taxonomy, classifier, compactor)
- Processes every corpus entry
- Compares `CanonicalEvent.Type` and `CanonicalEvent.Category` against expected values
- Reports per-category accuracy and overall accuracy
- Fails if overall top-1 accuracy < 80%
- Prints a confusion summary for misclassified entries (expected → got, with confidence) to guide taxonomy tuning

3.3 **Severity consistency test**

Verify that every corpus entry's output severity matches the expected severity. This validates that the per-leaf severity assignments in the taxonomy are correct and that the classifier maps logs to leaves with appropriate severity.

### Files changed

- `internal/engine/engine_test.go` — **new**, integration + accuracy tests

---

## Section 4: Confidence Threshold Tuning

**What:** Determine the optimal confidence threshold using corpus results. The threshold controls when a log is classified vs. marked UNCLASSIFIED.

### Tasks

4.1 **Threshold analysis**

After Section 3's accuracy test is working, run the corpus through the classifier with `threshold = 0.0` (accept everything) and record the confidence score for every entry:

- For correctly classified entries: what's the distribution of confidence scores?
- For misclassified entries: what are their confidence scores?
- Is there a natural gap between correct and incorrect classifications?

4.2 **Select threshold**

The ideal threshold maximizes correct classifications while minimizing misclassifications:
- If most correct classifications score >0.6 and most misclassifications score <0.4, threshold = 0.5 is fine
- If there's significant overlap, we may need to tune taxonomy descriptions (Section 5) before the threshold cleanly separates

4.3 **Make threshold configurable**

Currently `ConfidenceThreshold` is hardcoded to 0.5 in `config.go`. Add an environment variable:
- `LUMBER_CONFIDENCE_THRESHOLD` with default 0.5
- Parse as float64 in `config.Load()`

4.4 **Add threshold to test output**

Enhance the corpus accuracy test to report:
- Mean confidence for correct classifications
- Mean confidence for incorrect classifications
- Suggested threshold based on the gap

### Files changed

- `internal/config/config.go` — make threshold configurable via env var
- `internal/engine/engine_test.go` — threshold analysis reporting

---

## Section 5: Taxonomy Description Tuning

**What:** Iteratively improve leaf descriptions based on corpus accuracy results. This is the primary lever for improving classification quality.

### Tasks

5.1 **Analyze misclassifications**

From Section 3's accuracy test output, identify patterns:
- Which categories are most confused with each other?
- Are misclassifications due to ambiguous descriptions, or ambiguous log lines?
- Are any leaves systematically stealing classifications from others?

5.2 **Tune descriptions**

For each problematic category pair, adjust the leaf descriptions to increase semantic distance:
- Add discriminating keywords that the embedding model will pick up on
- Remove overlapping language between commonly confused categories
- Example: if `ERROR.timeout` and `ERROR.connection_failure` are confused, make descriptions more specific:
  - `timeout`: "Request deadline exceeded, operation timed out waiting for response, context deadline exceeded"
  - `connection_failure`: "TCP connection refused, DNS resolution failure, network unreachable, socket connection error"

5.3 **Re-validate**

After each round of description changes:
1. Re-run the corpus accuracy test
2. Check that overall accuracy improved
3. Check that fixing one category didn't break another
4. Repeat until >80% accuracy is reached or diminishing returns

### Design decisions

- **Descriptions are the primary tuning lever.** The embedding model is fixed. The taxonomy structure is fixed. The confidence threshold is a secondary lever. The description text is what determines the semantic position of each label in embedding space — it's the most impactful thing to change.
- **Iterative, not one-shot.** Expect 2–3 rounds of tuning. Each round: run corpus → identify worst categories → adjust descriptions → re-run.

### Files changed

- `internal/engine/taxonomy/default.go` — description text adjustments (iterative)

---

## Section 6: Edge Cases & Robustness

**What:** Ensure the pipeline handles degenerate inputs gracefully.

### Tasks

6.1 **Empty and whitespace-only logs**
- `Process(RawLog{Raw: ""})` — should return UNCLASSIFIED or a zero-confidence result, not crash
- `Process(RawLog{Raw: "   \n\t  "})` — same

6.2 **Very long logs**
- Logs exceeding the 128-token max sequence length — verify truncation works and classification is still reasonable (the first 128 tokens usually contain the signal)

6.3 **Binary/non-UTF8 content**
- Logs containing binary data, null bytes, or invalid UTF8 — should not crash the tokenizer or embedder

6.4 **Timestamp preservation**
- Verify that `RawLog.Timestamp` is faithfully copied to `CanonicalEvent.Timestamp` (not zeroed or modified)

6.5 **Metadata passthrough**
- Currently `RawLog.Metadata` is not surfaced in `CanonicalEvent`. Verify this is intentional for Phase 2 (metadata passthrough may be a Phase 4 concern).

### Files changed

- `internal/engine/engine_test.go` — edge case tests

---

## Implementation Order

```
Section 1 (taxonomy alignment)         — prerequisite for everything
    ↓
Section 2 (synthetic test corpus)       — needs finalized taxonomy for labeling
    ↓
Section 3 (engine pipeline tests)       — needs corpus + taxonomy
    ↓
Section 4 (threshold tuning)            — needs accuracy test results
    ↓
Section 5 (description tuning)          — needs misclassification analysis
    ↓
Section 6 (edge cases)                  — independent, can run in parallel with 4–5
```

Sections 1 and 2 are the bulk of the work. Sections 4 and 5 are iterative — they may interleave as we tune threshold and descriptions based on accuracy results. Section 6 can be done any time after Section 3.

---

## File Summary

New files:
- `internal/engine/testdata/corpus.json` — synthetic test corpus (~105 labeled log lines)
- `internal/engine/testdata/testdata.go` — corpus loader with `//go:embed`
- `internal/engine/engine_test.go` — integration tests, accuracy test, edge cases

Modified files:
- `internal/engine/taxonomy/default.go` — expanded taxonomy (34 → 42 leaves)
- `internal/engine/taxonomy/taxonomy_test.go` — updated fixtures
- `internal/config/config.go` — configurable confidence threshold

---

## Risks

1. **Classification accuracy may be hard to reach 80%.** The LEAF model is small (22M params) and log lines are terse. If accuracy plateaus below 80% after description tuning, options include:
   - Enriching embedding text with more context (e.g., embedding `"{Root}: {Leaf}: {Desc}"` or multiple description variants)
   - Reducing taxonomy granularity (merge leaves that can't be distinguished)
   - Trying a larger model (GTE-small at 33M params, same ONNX pipeline)

2. **Ambiguous logs may be genuinely ambiguous.** A log like `"error: service unavailable"` could be ERROR.dependency_error, REQUEST.server_error, or ERROR.connection_failure. The corpus should acknowledge legitimate ambiguity and not penalize the classifier for reasonable alternative classifications. Consider accepting top-2 accuracy for known-ambiguous entries.

3. **Taxonomy changes invalidate cached embeddings.** After changing descriptions, all taxonomy labels must be re-embedded. This happens automatically at startup via `taxonomy.New()`, but be aware during iterative tuning — the embedder must be re-initialized between runs.
