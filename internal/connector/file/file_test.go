package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaminocorp/lumber/internal/connector"
)

func TestStream_ReadsFile(t *testing.T) {
	path := writeTempFile(t, "one\ntwo\nthree\nfour\nfive\n")
	c := &Connector{}

	ch, err := c.Stream(context.Background(), cfgWithFile(path))
	if err != nil {
		t.Fatal(err)
	}

	var lines []string
	for raw := range ch {
		lines = append(lines, raw.Raw)
		if raw.Source != "file" {
			t.Errorf("expected source \"file\", got %q", raw.Source)
		}
	}

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d: %v", len(lines), lines)
	}
}

func TestStream_RespectsContextCancellation(t *testing.T) {
	// Large file — cancel before reading all.
	var b strings.Builder
	for i := 0; i < 10_000; i++ {
		b.WriteString("log line\n")
	}
	path := writeTempFile(t, b.String())
	c := &Connector{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := c.Stream(ctx, cfgWithFile(path))
	if err != nil {
		t.Fatal(err)
	}

	// Read a few, then cancel.
	count := 0
	for range ch {
		count++
		if count >= 5 {
			cancel()
			break
		}
	}
	// Drain remaining.
	for range ch {
	}

	if count < 5 {
		t.Errorf("expected at least 5 lines before cancel, got %d", count)
	}
}

func TestQuery_WithLimit(t *testing.T) {
	path := writeTempFile(t, "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n")
	c := &Connector{}

	results, err := c.Query(context.Background(), cfgWithFile(path), connector.QueryParams{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	expected := []string{"a", "b", "c"}
	for i, want := range expected {
		if results[i].Raw != want {
			t.Errorf("result %d: expected %q, got %q", i, want, results[i].Raw)
		}
	}
}

func TestStream_MissingFile(t *testing.T) {
	c := &Connector{}
	_, err := c.Stream(context.Background(), cfgWithFile("/nonexistent/path/to/file.log"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestStream_EmptyFile(t *testing.T) {
	path := writeTempFile(t, "")
	c := &Connector{}

	ch, err := c.Stream(context.Background(), cfgWithFile(path))
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events from empty file, got %d", count)
	}
}

func TestStream_FileMetadata(t *testing.T) {
	path := writeTempFile(t, "hello\n")
	c := &Connector{}

	ch, err := c.Stream(context.Background(), cfgWithFile(path))
	if err != nil {
		t.Fatal(err)
	}

	for raw := range ch {
		fname, ok := raw.Metadata["file"]
		if !ok {
			t.Fatal("expected \"file\" key in Metadata")
		}
		if fname != filepath.Base(path) {
			t.Errorf("expected filename %q in metadata, got %q", filepath.Base(path), fname)
		}
		if raw.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	}
}

func TestStream_MissingFilePath(t *testing.T) {
	c := &Connector{}
	_, err := c.Stream(context.Background(), connector.ConnectorConfig{
		Extra: map[string]string{},
	})
	if err == nil {
		t.Fatal("expected error for missing file path in config")
	}
	if !strings.Contains(err.Error(), "missing required config key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQuery_ReadsAllWithoutLimit(t *testing.T) {
	path := writeTempFile(t, "x\ny\nz\n")
	c := &Connector{}

	results, err := c.Query(context.Background(), cfgWithFile(path), connector.QueryParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results without limit, got %d", len(results))
	}
}

func TestQuery_TimeFiltersIgnored(t *testing.T) {
	path := writeTempFile(t, "a\nb\n")
	c := &Connector{}

	// Time filters should be ignored (file lines have no timestamp).
	results, err := c.Query(context.Background(), cfgWithFile(path), connector.QueryParams{
		Start: time.Now().Add(-time.Hour),
		End:   time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results (time filters ignored), got %d", len(results))
	}
}

// --- helpers ---

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func cfgWithFile(path string) connector.ConnectorConfig {
	return connector.ConnectorConfig{
		Extra: map[string]string{"file": path},
	}
}
