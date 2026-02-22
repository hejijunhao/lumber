# Classification Pipeline — Section 1: Taxonomy Alignment

**Completed:** 2026-02-21

## What changed

Expanded the default taxonomy from 34 leaves across 8 roots to **42 leaves across 8 roots**, aligning with the vision doc's full label set while retaining useful additions (DATA, SCHEDULED) from the Phase 1 implementation.

## Root-by-root changes

### ERROR (5 → 9 leaves)
- **Kept:** `connection_failure`, `timeout`, `validation_error`, `runtime_exception`
- **Renamed:** `auth_failure` — narrowed scope to authentication only (was covering both authn and authz)
- **Added:** `authorization_failure` (permission denied, RBAC), `out_of_memory` (OOM kills, heap exhaustion), `rate_limited` (moved from SECURITY, semantically an error), `dependency_error` (upstream/downstream failures)
- All descriptions rewritten for richer embedding quality with specific error message patterns

### REQUEST (3 → 5 leaves)
- **Replaced** `incoming_request`, `outgoing_request`, `response` with HTTP status class taxonomy: `success` (2xx), `client_error` (4xx), `server_error` (5xx), `redirect` (3xx), `slow_request`
- Rationale: HTTP status classes are more useful for classification — incoming/outgoing distinction is metadata, not semantics

### DEPLOY (6 → 7 leaves)
- **Added:** `rollback` — deployment rollback is a distinct event type
- Descriptions enriched with CI/CD-specific language

### SYSTEM (5 → 5 leaves, restructured)
- **Merged:** `startup` + `shutdown` → `process_lifecycle` — covers start, stop, restart, crash, signal handling
- **Renamed:** `resource_limit` → `resource_alert`, `scaling` → `scaling_event`
- **Added:** `config_change` (env var updates, feature flag toggles)

### SECURITY → ACCESS (4 → 5 leaves)
- **Renamed root** from SECURITY to ACCESS (matches vision doc, less ambiguous)
- **Kept:** `login_success`, `login_failure`
- **Removed:** `rate_limited` (moved to ERROR), `suspicious_activity` (too vague for embedding)
- **Added:** `session_expired`, `permission_change`, `api_key_event`

### PERFORMANCE (new, 5 leaves)
- **Added entire root** from vision doc: `latency_spike`, `throughput_drop`, `queue_backlog`, `cache_event`, `db_slow_query`
- `cache_event` consolidates the old `cache_hit`/`cache_miss` from DATA (individual cache ops are noise; the signal is patterns)

### DATA (4 → 3 leaves)
- **Renamed:** `query` → `query_executed` for clarity
- **Added:** `replication` (data sync, backup events)
- **Removed:** `cache_hit`, `cache_miss` (consolidated into PERFORMANCE.cache_event)

### SCHEDULED (unchanged, 3 leaves)
- No structural changes, descriptions enriched

### APPLICATION (removed)
- **Deleted entire root.** `info`/`warning`/`debug` are severity levels, not categories. Logs should classify into a specific semantic category. Genuinely uncategorizable logs will hit UNCLASSIFIED via the confidence threshold.

## Description writing approach

Leaf descriptions are the texts that get embedded and compared against log lines via cosine similarity. They were written with these goals:
- **Specific error patterns:** Include concrete phrases that appear in real logs (e.g., "TCP connection refused", "HTTP 429 Too Many Requests", "OOM kill")
- **Disambiguation:** Commonly confused pairs use deliberately non-overlapping language (e.g., `auth_failure` emphasizes "credentials, password, token" while `authorization_failure` emphasizes "permission, scope, RBAC")
- **Length:** Each description is 80-120 chars with 3-5 comma-separated patterns, giving the embedding model enough semantic signal

## Severity assignments

Every leaf has an explicit severity following the Phase 1 pattern:
- `error`: connection_failure, auth_failure, authorization_failure, timeout, runtime_exception, out_of_memory, dependency_error, server_error, build_failed, deploy_failed, cron_failed
- `warning`: validation_error, rate_limited, client_error, slow_request, rollback, resource_alert, login_failure, latency_spike, throughput_drop, queue_backlog, db_slow_query
- `info`: success, redirect, build_started, build_succeeded, deploy_started, deploy_succeeded, health_check, scaling_event, process_lifecycle, config_change, login_success, session_expired, permission_change, api_key_event, cache_event, query_executed, migration, replication, cron_started, cron_completed
- `debug`: none (removed with APPLICATION root)

## Tests

### Updated
- `taxonomy_test.go` — `TestNewPreEmbeds` fixture updated (`startup` → `process_lifecycle`)

### Added
- `TestDefaultRootsLeafCount` — verifies 8 roots, 42 total leaves, correct per-root counts
- `TestDefaultRootsSeverity` — every leaf has a valid severity (error/warning/info/debug)
- `TestDefaultRootsDescriptions` — no empty descriptions, minimum 20-char length for embedding quality

All 7 tests pass.

## Files changed

- `internal/engine/taxonomy/default.go` — full taxonomy rewrite (34 → 42 leaves)
- `internal/engine/taxonomy/taxonomy_test.go` — updated fixture + 3 new validation tests
