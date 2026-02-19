package embedder

import (
	"os"
	"testing"
)

const testProjectionPath = "../../../models/2_Dense/model.safetensors"

func skipIfNoProjection(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(testProjectionPath); os.IsNotExist(err) {
		t.Skip("projection weights not found; run 'make download-model' first")
	}
}

func TestLoadProjection(t *testing.T) {
	skipIfNoProjection(t)

	proj, err := loadProjection(testProjectionPath)
	if err != nil {
		t.Fatalf("failed to load projection: %v", err)
	}

	if proj.inDim != 384 {
		t.Errorf("expected inDim=384, got %d", proj.inDim)
	}
	if proj.outDim != 1024 {
		t.Errorf("expected outDim=1024, got %d", proj.outDim)
	}
	if len(proj.weights) != 1024*384 {
		t.Errorf("expected %d weights, got %d", 1024*384, len(proj.weights))
	}

	// Spot-check: weights should not be all zeros.
	allZero := true
	for _, w := range proj.weights[:100] {
		if w != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("first 100 weights are all zeros — file may be corrupt")
	}
}

func TestProjectionApply(t *testing.T) {
	skipIfNoProjection(t)

	proj, err := loadProjection(testProjectionPath)
	if err != nil {
		t.Fatalf("failed to load projection: %v", err)
	}

	// Apply to a simple input vector.
	input := make([]float32, 384)
	for i := range input {
		input[i] = 1.0 / 384.0
	}

	out := proj.apply(input)
	if len(out) != 1024 {
		t.Fatalf("expected 1024-dim output, got %d", len(out))
	}

	// Output should not be all zeros.
	allZero := true
	for _, v := range out {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("projection output is all zeros")
	}
}

func TestEmbedEndToEnd(t *testing.T) {
	skipIfNoModel(t)
	skipIfNoVocab(t)
	skipIfNoProjection(t)

	emb, err := New(testModelPath, testVocabPath, testProjectionPath)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	if emb.EmbedDim() != 1024 {
		t.Errorf("expected EmbedDim()=1024, got %d", emb.EmbedDim())
	}

	vec, err := emb.Embed("hello world")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vec) != 1024 {
		t.Fatalf("expected 1024-dim vector, got %d", len(vec))
	}

	// Vector should not be all zeros.
	allZero := true
	for _, v := range vec {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("embedding is all zeros")
	}

	t.Logf("first 5 dims: %v", vec[:5])
}

func TestEmbedBatchEndToEnd(t *testing.T) {
	skipIfNoModel(t)
	skipIfNoVocab(t)
	skipIfNoProjection(t)

	emb, err := New(testModelPath, testVocabPath, testProjectionPath)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	texts := []string{
		"connection timeout to database",
		"deploy succeeded in 12 seconds",
	}
	vecs, err := emb.EmbedBatch(texts)
	if err != nil {
		t.Fatalf("EmbedBatch failed: %v", err)
	}
	if len(vecs) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vecs))
	}
	for i, vec := range vecs {
		if len(vec) != 1024 {
			t.Errorf("vector %d: expected 1024 dims, got %d", i, len(vec))
		}
	}

	// The two embeddings should be different (different semantic content).
	same := true
	for i := range vecs[0] {
		if vecs[0][i] != vecs[1][i] {
			same = false
			break
		}
	}
	if same {
		t.Error("batch embeddings are identical — pooling or projection may be broken")
	}
}

func TestEmbedBatchEmpty(t *testing.T) {
	skipIfNoModel(t)
	skipIfNoVocab(t)
	skipIfNoProjection(t)

	emb, err := New(testModelPath, testVocabPath, testProjectionPath)
	if err != nil {
		t.Fatalf("failed to create embedder: %v", err)
	}
	defer emb.Close()

	vecs, err := emb.EmbedBatch(nil)
	if err != nil {
		t.Fatalf("EmbedBatch(nil) failed: %v", err)
	}
	if vecs != nil {
		t.Errorf("expected nil for empty batch, got %v", vecs)
	}
}
