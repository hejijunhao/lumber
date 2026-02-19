package embedder

import (
	"os"
	"testing"
)

const testModelPath = "../../../models/model_quantized.onnx"

func skipIfNoModel(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(testModelPath); os.IsNotExist(err) {
		t.Skip("model files not found; run 'make download-model' first")
	}
}

func TestONNXSessionLoad(t *testing.T) {
	skipIfNoModel(t)

	sess, err := newONNXSession(testModelPath)
	if err != nil {
		t.Fatalf("failed to load ONNX session: %v", err)
	}
	defer sess.close()

	if sess.embedDim <= 0 {
		t.Errorf("expected positive embedDim, got %d", sess.embedDim)
	}

	t.Logf("input names: %v", sess.inputNames)
	t.Logf("output name: %s", sess.outputName)
	t.Logf("embed dim: %d", sess.embedDim)
}

func TestONNXInference(t *testing.T) {
	skipIfNoModel(t)

	sess, err := newONNXSession(testModelPath)
	if err != nil {
		t.Fatalf("failed to load ONNX session: %v", err)
	}
	defer sess.close()

	// Minimal input: [CLS]=101, [SEP]=102, rest padding.
	const seqLen = 8
	inputIDs := []int64{101, 102, 0, 0, 0, 0, 0, 0}
	attentionMask := []int64{1, 1, 0, 0, 0, 0, 0, 0}
	tokenTypeIDs := make([]int64, seqLen)

	out, err := sess.infer(inputIDs, attentionMask, tokenTypeIDs, 1, seqLen)
	if err != nil {
		t.Fatalf("inference failed: %v", err)
	}

	expectedLen := seqLen * int(sess.embedDim)
	if len(out) != expectedLen {
		t.Fatalf("expected output length %d, got %d", expectedLen, len(out))
	}

	// Verify we got actual values, not all zeros.
	allZero := true
	for _, v := range out {
		if v != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("output is all zeros — model may not be producing real embeddings")
	}

	t.Logf("first 5 values of CLS token embedding: %v", out[:5])
}

func TestONNXBatchInference(t *testing.T) {
	skipIfNoModel(t)

	sess, err := newONNXSession(testModelPath)
	if err != nil {
		t.Fatalf("failed to load ONNX session: %v", err)
	}
	defer sess.close()

	const batchSize, seqLen = 2, 8

	// Two sequences: [CLS] hello [SEP] pad... and [CLS] world [SEP] pad...
	inputIDs := []int64{
		101, 7592, 102, 0, 0, 0, 0, 0, // "hello"
		101, 2088, 102, 0, 0, 0, 0, 0, // "world"
	}
	attentionMask := []int64{
		1, 1, 1, 0, 0, 0, 0, 0,
		1, 1, 1, 0, 0, 0, 0, 0,
	}
	tokenTypeIDs := make([]int64, batchSize*seqLen)

	out, err := sess.infer(inputIDs, attentionMask, tokenTypeIDs, batchSize, seqLen)
	if err != nil {
		t.Fatalf("batch inference failed: %v", err)
	}

	expectedLen := batchSize * seqLen * int(sess.embedDim)
	if len(out) != expectedLen {
		t.Fatalf("expected output length %d, got %d", expectedLen, len(out))
	}

	t.Logf("batch inference produced %d float32 values", len(out))
}
