package embedder

import "fmt"

// Embedder produces vector embeddings from text.
type Embedder interface {
	Embed(text string) ([]float32, error)
	EmbedBatch(texts []string) ([][]float32, error)
	Close() error
}

// ONNXEmbedder wraps the ONNX runtime for local embedding inference.
type ONNXEmbedder struct {
	session *onnxSession
}

// New creates an ONNXEmbedder by loading the ONNX model at modelPath.
// The ONNX Runtime shared library is expected at models/libonnxruntime.so
// (same directory as the model file).
func New(modelPath string) (*ONNXEmbedder, error) {
	sess, err := newONNXSession(modelPath)
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}
	return &ONNXEmbedder{session: sess}, nil
}

// EmbedDim returns the embedding dimensionality of the loaded model.
func (e *ONNXEmbedder) EmbedDim() int {
	return int(e.session.embedDim)
}

func (e *ONNXEmbedder) Embed(text string) ([]float32, error) {
	// Stub — requires tokenizer (Section 2) and pooling (Section 3).
	return nil, fmt.Errorf("embedder: Embed not yet wired (needs tokenizer)")
}

func (e *ONNXEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	// Stub — requires tokenizer (Section 2) and pooling (Section 3).
	return nil, fmt.Errorf("embedder: EmbedBatch not yet wired (needs tokenizer)")
}

// Close releases ONNX Runtime resources.
func (e *ONNXEmbedder) Close() error {
	if e.session != nil {
		return e.session.close()
	}
	return nil
}
