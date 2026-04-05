package classifier

import (
	"math"
	"testing"

	"github.com/kaminocorp/lumber/internal/model"
)

func label(path string, vec []float32) model.EmbeddedLabel {
	return model.EmbeddedLabel{Path: path, Vector: vec, Severity: "error"}
}

func TestClassify_BestMatch(t *testing.T) {
	labels := []model.EmbeddedLabel{
		label("ERROR.connection_failure", []float32{1, 0, 0}),
		label("ERROR.timeout", []float32{0, 1, 0}),
		label("REQUEST.success", []float32{0, 0, 1}),
	}
	c := New(0.5)

	// Vector close to connection_failure.
	result := c.Classify([]float32{0.9, 0.1, 0.0}, labels)
	if result.Label.Path != "ERROR.connection_failure" {
		t.Errorf("got %q, want ERROR.connection_failure", result.Label.Path)
	}
	if result.Confidence < 0.9 {
		t.Errorf("confidence %f unexpectedly low", result.Confidence)
	}
}

func TestClassify_BelowThreshold(t *testing.T) {
	labels := []model.EmbeddedLabel{
		label("ERROR.connection_failure", []float32{1, 0, 0}),
	}
	c := New(0.99)

	// Somewhat aligned but below a strict threshold.
	result := c.Classify([]float32{0.7, 0.7, 0.0}, labels)
	if result.Label.Path != "UNCLASSIFIED" {
		t.Errorf("got %q, want UNCLASSIFIED", result.Label.Path)
	}
}

func TestClassify_EmptyLabels(t *testing.T) {
	c := New(0.5)
	result := c.Classify([]float32{1, 0, 0}, nil)
	if result.Label.Path != "UNCLASSIFIED" {
		t.Errorf("got %q, want UNCLASSIFIED", result.Label.Path)
	}
	if result.Confidence != 0 {
		t.Errorf("confidence %f, want 0", result.Confidence)
	}
}

func TestClassify_ZeroVector(t *testing.T) {
	labels := []model.EmbeddedLabel{
		label("ERROR.timeout", []float32{1, 0, 0}),
	}
	c := New(0.5)

	result := c.Classify([]float32{0, 0, 0}, labels)
	if result.Label.Path != "UNCLASSIFIED" {
		t.Errorf("got %q, want UNCLASSIFIED for zero vector", result.Label.Path)
	}
}

func TestClassify_TieBreaking(t *testing.T) {
	// Two labels with identical vectors — first one wins (stable).
	labels := []model.EmbeddedLabel{
		label("A", []float32{1, 0}),
		label("B", []float32{1, 0}),
	}
	c := New(0.0)

	result := c.Classify([]float32{1, 0}, labels)
	if result.Label.Path != "A" {
		t.Errorf("got %q, want A (first match wins on tie)", result.Label.Path)
	}
}

func TestClassify_ZeroThreshold(t *testing.T) {
	labels := []model.EmbeddedLabel{
		label("A", []float32{1, 0}),
	}
	c := New(0.0)

	// With threshold 0, any positive similarity classifies.
	result := c.Classify([]float32{0.5, 0.5}, labels)
	if result.Label.Path != "A" {
		t.Errorf("got %q, want A with zero threshold", result.Label.Path)
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	sim := cosineSimilarity([]float32{1, 0}, []float32{0, 1})
	if math.Abs(sim) > 1e-6 {
		t.Errorf("orthogonal vectors: got %f, want 0", sim)
	}
}

func TestCosineSimilarity_Identical(t *testing.T) {
	sim := cosineSimilarity([]float32{3, 4}, []float32{3, 4})
	if math.Abs(sim-1.0) > 1e-6 {
		t.Errorf("identical vectors: got %f, want 1", sim)
	}
}

func TestCosineSimilarity_Opposite(t *testing.T) {
	sim := cosineSimilarity([]float32{1, 0}, []float32{-1, 0})
	if math.Abs(sim+1.0) > 1e-6 {
		t.Errorf("opposite vectors: got %f, want -1", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	sim := cosineSimilarity([]float32{1, 0}, []float32{1, 0, 0})
	if sim != 0 {
		t.Errorf("different lengths: got %f, want 0", sim)
	}
}

func TestCosineSimilarity_Empty(t *testing.T) {
	sim := cosineSimilarity([]float32{}, []float32{})
	if sim != 0 {
		t.Errorf("empty: got %f, want 0", sim)
	}
}

func TestCosineSimilarity_ZeroNorm(t *testing.T) {
	sim := cosineSimilarity([]float32{0, 0}, []float32{1, 0})
	if sim != 0 {
		t.Errorf("zero norm: got %f, want 0", sim)
	}
}
