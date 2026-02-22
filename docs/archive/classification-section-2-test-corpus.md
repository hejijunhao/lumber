# Classification Pipeline — Section 2: Synthetic Test Corpus

**Completed:** 2026-02-21

## What was built

A synthetic test corpus of **104 labeled log lines** covering all 42 taxonomy leaves, plus a Go loader with `//go:embed` and validation tests.

## Files created

### `internal/engine/testdata/corpus.json`

104 entries, each with:
- `raw` — realistic log line text
- `expected_type` — root category (ERROR, REQUEST, etc.)
- `expected_category` — leaf category (connection_failure, success, etc.)
- `expected_severity` — expected severity from taxonomy (error, warning, info)
- `description` — human-readable label for the entry

### `internal/engine/testdata/testdata.go`

Corpus loader using `//go:embed corpus.json`. Exports:
- `CorpusEntry` struct matching the JSON schema
- `LoadCorpus() ([]CorpusEntry, error)` — parses embedded JSON

### `internal/engine/testdata/testdata_test.go`

3 validation tests:
- `TestLoadCorpus` — JSON parses, all required fields non-empty
- `TestCorpusCoverage` — all 42 taxonomy leaves have >= 2 entries
- `TestCorpusSeverityValues` — all severity values are valid (error/warning/info/debug)

## Corpus coverage

| Root | Entries | Leaves covered |
|------|---------|----------------|
| ERROR | 26 | 9/9 |
| REQUEST | 13 | 5/5 |
| DEPLOY | 14 | 7/7 |
| SYSTEM | 13 | 5/5 |
| ACCESS | 13 | 5/5 |
| PERFORMANCE | 13 | 5/5 |
| DATA | 8 | 3/3 |
| SCHEDULED | 6 | 3/3 |
| **Total** | **104** | **42/42** |

Every leaf has 2–3 entries. Most have entries in at least 2 distinct log formats.

## Format diversity

The corpus uses a mix of:
- **JSON structured logs** — `{"level":"error","msg":"..."}` style
- **Plain text with timestamps** — `ERROR [2026-02-19 12:00:00] ...`
- **Key=value structured** — `level=error msg="..." service=api`
- **Pipe-delimited** — `2026-02-19T12:00:00Z | WARN | ...`
- **Apache/nginx combined log format** — for HTTP request entries
- **Stack traces** — Go panic, Java NullPointerException, JavaScript TypeError
- **System/kernel logs** — Linux OOM killer output
- **CI/CD output** — build/deploy event messages

## Design decisions

- **2–3 entries per leaf, not more:** Enough for discrimination testing without bloating the corpus. Categories that prove weak in Section 3 accuracy testing can have entries added later.
- **Format diversity > volume:** Two very different log formats for the same category tests more than five similar formats.
- **Realistic but unambiguous for v1:** Most entries are clearly in one category. Known-ambiguous cases (e.g., a failed cron job that's also a connection failure) are deferred until Section 5 when we tune for edge cases.
- **Severity matches taxonomy:** Every entry's `expected_severity` matches the severity assigned to its taxonomy leaf in `default.go`.
