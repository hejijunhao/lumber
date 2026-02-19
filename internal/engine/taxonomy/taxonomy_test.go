package taxonomy

import (
	"fmt"
	"testing"

	"github.com/crimson-sun/lumber/internal/model"
)

// mockEmbedder returns deterministic vectors for testing.
type mockEmbedder struct {
	dim   int
	calls int
}

func (m *mockEmbedder) Embed(text string) ([]float32, error) {
	m.calls++
	vec := make([]float32, m.dim)
	vec[0] = float32(m.calls)
	return vec, nil
}

func (m *mockEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	m.calls++
	vecs := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dim)
		vec[0] = float32(i + 1)
		vecs[i] = vec
	}
	return vecs, nil
}

func (m *mockEmbedder) Close() error { return nil }

// failEmbedder always returns an error.
type failEmbedder struct{}

func (f *failEmbedder) Embed(string) ([]float32, error)        { return nil, fmt.Errorf("embed failed") }
func (f *failEmbedder) EmbedBatch([]string) ([][]float32, error) { return nil, fmt.Errorf("embed failed") }
func (f *failEmbedder) Close() error                             { return nil }

func TestNewPreEmbeds(t *testing.T) {
	roots := []*model.TaxonomyNode{
		{
			Name: "ERROR",
			Desc: "Errors",
			Children: []*model.TaxonomyNode{
				{Name: "timeout", Desc: "Request timeout", Severity: "error"},
				{Name: "connection_failure", Desc: "Connection failed", Severity: "error"},
			},
		},
		{
			Name: "SYSTEM",
			Desc: "System events",
			Children: []*model.TaxonomyNode{
				{Name: "startup", Desc: "Service startup", Severity: "info"},
			},
		},
	}

	emb := &mockEmbedder{dim: 4}
	tax, err := New(roots, emb)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	labels := tax.Labels()
	if len(labels) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(labels))
	}

	// Verify paths.
	wantPaths := []string{"ERROR.timeout", "ERROR.connection_failure", "SYSTEM.startup"}
	for i, want := range wantPaths {
		if labels[i].Path != want {
			t.Errorf("label[%d].Path = %q, want %q", i, labels[i].Path, want)
		}
	}

	// Verify severities.
	wantSeverities := []string{"error", "error", "info"}
	for i, want := range wantSeverities {
		if labels[i].Severity != want {
			t.Errorf("label[%d].Severity = %q, want %q", i, labels[i].Severity, want)
		}
	}

	// Verify vectors are populated and have correct dimension.
	for i, label := range labels {
		if len(label.Vector) != 4 {
			t.Errorf("label[%d].Vector length = %d, want 4", i, len(label.Vector))
		}
		if label.Vector[0] == 0 {
			t.Errorf("label[%d].Vector[0] = 0, expected non-zero", i)
		}
	}
}

func TestNewEmptyRoots(t *testing.T) {
	emb := &mockEmbedder{dim: 4}
	tax, err := New(nil, emb)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if len(tax.Labels()) != 0 {
		t.Errorf("expected 0 labels, got %d", len(tax.Labels()))
	}
}

func TestNewNoLeaves(t *testing.T) {
	roots := []*model.TaxonomyNode{
		{Name: "ERROR", Desc: "Errors"},
	}
	emb := &mockEmbedder{dim: 4}
	tax, err := New(roots, emb)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if len(tax.Labels()) != 0 {
		t.Errorf("expected 0 labels for root-only nodes, got %d", len(tax.Labels()))
	}
}

func TestNewEmbedError(t *testing.T) {
	roots := []*model.TaxonomyNode{
		{
			Name:     "ERROR",
			Desc:     "Errors",
			Children: []*model.TaxonomyNode{{Name: "timeout", Desc: "Timeout"}},
		},
	}
	_, err := New(roots, &failEmbedder{})
	if err == nil {
		t.Fatal("expected error from failing embedder")
	}
}
