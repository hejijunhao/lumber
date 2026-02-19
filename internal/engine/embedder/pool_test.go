package embedder

import (
	"math"
	"testing"
)

func TestMeanPool(t *testing.T) {
	// 1 sample, seqLen=3, dim=2.
	// Token hidden states: [1, 2], [3, 4], [5, 6]
	// Mask: [1, 1, 0] — only first two tokens are real.
	// Expected: [(1+3)/2, (2+4)/2] = [2, 3]
	hidden := []float32{1, 2, 3, 4, 5, 6}
	mask := []int64{1, 1, 0}

	out := meanPool(hidden, mask, 1, 3, 2)

	if len(out) != 2 {
		t.Fatalf("expected 2 values, got %d", len(out))
	}
	if !closeEnough(out[0], 2.0) || !closeEnough(out[1], 3.0) {
		t.Errorf("expected [2, 3], got %v", out)
	}
}

func TestMeanPoolBatch(t *testing.T) {
	// 2 samples, seqLen=2, dim=2.
	// Sample 0: [10, 20], [30, 40], mask=[1, 1] → [20, 30]
	// Sample 1: [5, 15], [0, 0],   mask=[1, 0] → [5, 15]
	hidden := []float32{10, 20, 30, 40, 5, 15, 0, 0}
	mask := []int64{1, 1, 1, 0}

	out := meanPool(hidden, mask, 2, 2, 2)

	if len(out) != 4 {
		t.Fatalf("expected 4 values, got %d", len(out))
	}
	if !closeEnough(out[0], 20.0) || !closeEnough(out[1], 30.0) {
		t.Errorf("sample 0: expected [20, 30], got [%f, %f]", out[0], out[1])
	}
	if !closeEnough(out[2], 5.0) || !closeEnough(out[3], 15.0) {
		t.Errorf("sample 1: expected [5, 15], got [%f, %f]", out[2], out[3])
	}
}

func TestMeanPoolAllPadding(t *testing.T) {
	// All padding — should return zeros.
	hidden := []float32{1, 2, 3, 4}
	mask := []int64{0, 0}

	out := meanPool(hidden, mask, 1, 2, 2)

	for i, v := range out {
		if v != 0 {
			t.Errorf("out[%d] = %f, want 0 (all-padding case)", i, v)
		}
	}
}

func closeEnough(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-6
}
