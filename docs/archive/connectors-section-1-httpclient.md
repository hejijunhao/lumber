# Section 1: Shared HTTP Client — Completion Notes

## What Was Done

### 1.1 — `internal/connector/httpclient/httpclient.go`

Created a reusable HTTP client package used as the foundation for all connector implementations.

**Public API:**

- **`Client` struct** — holds base URL, bearer token, and an `*http.Client` with configurable timeout (default 30s)
- **`New(baseURL, token, ...Option) *Client`** — constructor with functional options
- **`WithTimeout(d) Option`** — overrides the default 30s HTTP client timeout
- **`GetJSON(ctx, path, query, dest) error`** — sends an authenticated GET request, unmarshals JSON response into `dest`. Handles:
  - URL construction: `baseURL + path + ?query`
  - `Authorization: Bearer {token}` header on every request
  - Non-2xx responses returned as `*APIError` (status code + first 512 bytes of body)
  - 429 retry with `Retry-After` header parsing (seconds as integer)
  - 5xx retry with exponential backoff (1s, 2s, 4s)
  - Max 3 retries (4 total attempts)
  - Context-aware retry sleep via `time.NewTimer` + `select` on `ctx.Done()`
- **`APIError` struct** — exported type with `StatusCode int` and `Body string`, implements `error` interface. Has an unexported `retryAfter string` field used internally for 429 retry delay tracking.

**Implementation details:**
- Zero external dependencies — uses only `net/http`, `encoding/json`, `net/url`, `io`, `context`, `time`, `strconv`, `fmt`
- `backoffDelay(attempt, lastErr)` helper computes retry wait: parses `Retry-After` for 429s (falls through to exponential if header is absent or unparseable), exponential for 5xx
- Non-retryable errors (4xx other than 429) return immediately without retry

### 1.2 — Tests

Created `internal/connector/httpclient/httpclient_test.go` with 8 tests, all using `httptest.NewServer`:

| Test | What it verifies |
|------|-----------------|
| `TestGetJSON_Success` | 200 with valid JSON unmarshals correctly |
| `TestGetJSON_BearerAuth` | `Authorization: Bearer xxx` header reaches the server |
| `TestGetJSON_QueryParams` | Query string constructed and encoded correctly |
| `TestGetJSON_APIError` | 400 returns `*APIError` with correct status and body |
| `TestGetJSON_RateLimit_RetryAfter` | 429 with `Retry-After: 1` → retries after ~1s → succeeds on second call |
| `TestGetJSON_RetryOn5xx` | 503 → retries → succeeds on second call |
| `TestGetJSON_ContextCancelled` | Cancelled context returns `context.Canceled` promptly during retry wait |
| `TestGetJSON_MaxRetriesExceeded` | 429 on every call → `*APIError` after 4 total attempts (1 + 3 retries) |

All 8 tests pass. No ONNX model required.

## Design Decisions

- **Single `GetJSON` method** rather than separate methods per HTTP verb. All three provider APIs (Vercel, Fly.io, Supabase) use GET requests. POST/PUT can be added later if needed.
- **`Retry-After` tracked on `APIError` internally** via an unexported field. This avoids leaking implementation detail to callers while allowing the retry loop to access the header value across iterations without carrying response headers separately.
- **Body truncation at 512 bytes** — prevents large error responses from consuming memory. The truncated body is sufficient for debugging.
- **No logging** — retry behavior is observable via timing and error return values. Connectors can add their own logging around `GetJSON` calls.

## Files

| File | Action |
|------|--------|
| `internal/connector/httpclient/httpclient.go` | Created |
| `internal/connector/httpclient/httpclient_test.go` | Created |

## Verification

```
$ go test ./internal/connector/httpclient/... -v -count=1
=== RUN   TestGetJSON_Success          --- PASS
=== RUN   TestGetJSON_BearerAuth       --- PASS
=== RUN   TestGetJSON_QueryParams      --- PASS
=== RUN   TestGetJSON_APIError         --- PASS
=== RUN   TestGetJSON_RateLimit_RetryAfter --- PASS (1.00s)
=== RUN   TestGetJSON_RetryOn5xx       --- PASS (1.00s)
=== RUN   TestGetJSON_ContextCancelled --- PASS
=== RUN   TestGetJSON_MaxRetriesExceeded --- PASS (7.01s)
PASS

$ go build ./cmd/lumber  # compiles cleanly
```
