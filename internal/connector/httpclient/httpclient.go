package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client is an HTTP client with Bearer auth, base URL, and retry logic.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// APIError represents a non-2xx HTTP response.
type APIError struct {
	StatusCode int
	Body       string // first 512 bytes
	retryAfter string // internal: Retry-After header value for 429s
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// Option configures Client behavior.
type Option func(*Client)

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// New creates a Client with Bearer auth and a base URL.
func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

const maxRetries = 3

// GetJSON sends a GET request and unmarshals the JSON response into dest.
// Returns *APIError for non-2xx responses. Retries on 429 (with Retry-After)
// and 5xx (with exponential backoff: 1s, 2s, 4s). Max 3 retries.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, dest any) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var lastErr *APIError
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			wait := backoffDelay(attempt, lastErr)
			t := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return json.Unmarshal(body, dest)
		}

		bodyStr := string(body)
		if len(bodyStr) > 512 {
			bodyStr = bodyStr[:512]
		}

		apiErr := &APIError{StatusCode: resp.StatusCode, Body: bodyStr}

		if resp.StatusCode == 429 {
			apiErr.retryAfter = resp.Header.Get("Retry-After")
			lastErr = apiErr
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = apiErr
			continue
		}

		return apiErr
	}

	return lastErr
}

// backoffDelay returns the wait duration before a retry attempt.
func backoffDelay(attempt int, lastErr *APIError) time.Duration {
	if lastErr != nil && lastErr.StatusCode == 429 && lastErr.retryAfter != "" {
		if secs, err := strconv.Atoi(lastErr.retryAfter); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	// Exponential backoff: 1s, 2s, 4s
	return time.Duration(1<<(attempt-1)) * time.Second
}
