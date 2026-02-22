package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetJSON_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"lumber","version":1}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var dest struct {
		Name    string `json:"name"`
		Version int    `json:"version"`
	}
	err := c.GetJSON(context.Background(), "/info", nil, &dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest.Name != "lumber" || dest.Version != 1 {
		t.Fatalf("unexpected result: %+v", dest)
	}
}

func TestGetJSON_BearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "secret-token-123")
	err := c.GetJSON(context.Background(), "/", nil, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer secret-token-123" {
		t.Fatalf("expected 'Bearer secret-token-123', got %q", gotAuth)
	}
}

func TestGetJSON_QueryParams(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	q := make(map[string][]string)
	q["from"] = []string{"100"}
	q["to"] = []string{"200"}
	err := c.GetJSON(context.Background(), "/logs", q, &struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// url.Values.Encode sorts keys alphabetically
	if gotQuery != "from=100&to=200" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}
}

func TestGetJSON_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	err := c.GetJSON(context.Background(), "/bad", nil, &struct{}{})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Fatalf("expected status 400, got %d", apiErr.StatusCode)
	}
	if apiErr.Body != `{"error":"bad request"}` {
		t.Fatalf("unexpected body: %q", apiErr.Body)
	}
}

func TestGetJSON_RateLimit_RetryAfter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			w.Write([]byte(`rate limited`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var dest struct {
		OK bool `json:"ok"`
	}
	start := time.Now()
	err := c.GetJSON(context.Background(), "/", nil, &dest)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dest.OK {
		t.Fatal("expected ok=true")
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("expected ~1s retry delay, got %v", elapsed)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", calls.Load())
	}
}

func TestGetJSON_RetryOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			w.Write([]byte(`service unavailable`))
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	var dest struct {
		OK bool `json:"ok"`
	}
	err := c.GetJSON(context.Background(), "/", nil, &dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dest.OK {
		t.Fatal("expected ok=true")
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", calls.Load())
	}
}

func TestGetJSON_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`rate limited`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the retry sleep is interrupted.
	cancel()

	c := New(srv.URL, "tok")
	err := c.GetJSON(ctx, "/", nil, &struct{}{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestGetJSON_MaxRetriesExceeded(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
		w.Write([]byte(`rate limited`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	err := c.GetJSON(context.Background(), "/", nil, &struct{}{})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", apiErr.StatusCode)
	}
	// 1 initial + 3 retries = 4 total calls
	if calls.Load() != 4 {
		t.Fatalf("expected 4 calls, got %d", calls.Load())
	}
}
