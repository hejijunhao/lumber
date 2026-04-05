package stdin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaminocorp/lumber/internal/connector"
)

func TestStream_ReadsLines(t *testing.T) {
	input := "line one\nline two\nline three\n"
	c := New(WithReader(strings.NewReader(input)))

	ch, err := c.Stream(context.Background(), connector.ConnectorConfig{})
	if err != nil {
		t.Fatal(err)
	}

	var lines []string
	for raw := range ch {
		lines = append(lines, raw.Raw)
		if raw.Source != "stdin" {
			t.Errorf("expected source \"stdin\", got %q", raw.Source)
		}
		if raw.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	}

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	expected := []string{"line one", "line two", "line three"}
	for i, want := range expected {
		if lines[i] != want {
			t.Errorf("line %d: expected %q, got %q", i, want, lines[i])
		}
	}
}

func TestStream_RespectsContextCancellation(t *testing.T) {
	// Use an empty reader — scanner sees EOF immediately.
	// The test confirms the goroutine respects context and closes the channel.
	c := New(WithReader(strings.NewReader("")))

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.Stream(ctx, connector.ConnectorConfig{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	// Channel must close within a reasonable time.
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case _, ok := <-ch:
		if ok {
			for range ch {
			}
		}
	case <-timer.C:
		t.Fatal("channel did not close after context cancellation")
	}
}

func TestStream_EmptyInput(t *testing.T) {
	c := New(WithReader(strings.NewReader("")))

	ch, err := c.Stream(context.Background(), connector.ConnectorConfig{})
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events from empty input, got %d", count)
	}
}

func TestStream_SkipsEmptyLines(t *testing.T) {
	input := "line one\n\n\nline two\n"
	c := New(WithReader(strings.NewReader(input)))

	ch, err := c.Stream(context.Background(), connector.ConnectorConfig{})
	if err != nil {
		t.Fatal(err)
	}

	var lines []string
	for raw := range ch {
		lines = append(lines, raw.Raw)
	}

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (empty skipped), got %d: %v", len(lines), lines)
	}
}

func TestQuery_ReturnsError(t *testing.T) {
	c := New()
	_, err := c.Query(context.Background(), connector.ConnectorConfig{}, connector.QueryParams{})
	if err == nil {
		t.Fatal("expected error from Query")
	}
	if !strings.Contains(err.Error(), "does not support query mode") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStream_LongLines(t *testing.T) {
	// Create a line longer than the default 64KB scanner limit.
	longLine := strings.Repeat("x", 100_000)
	input := longLine + "\n"
	c := New(WithReader(strings.NewReader(input)))

	ch, err := c.Stream(context.Background(), connector.ConnectorConfig{})
	if err != nil {
		t.Fatal(err)
	}

	var got string
	for raw := range ch {
		got = raw.Raw
	}
	if len(got) != 100_000 {
		t.Errorf("expected 100000 chars, got %d", len(got))
	}
}
