package embedder

import "errors"

var errNotImplemented = errors.New("embedder: not implemented")

// Embedder produces vector embeddings from text.
type Embedder interface {
	Embed(text string) ([]float32, error)
	EmbedBatch(texts []string) ([][]float32, error)
}

// ONNXEmbedder wraps the ONNX runtime for local embedding inference.
type ONNXEmbedder struct {
	ModelPath string
}

func New(modelPath string) (*ONNXEmbedder, error) {
	return &ONNXEmbedder{ModelPath: modelPath}, nil
}

func (e *ONNXEmbedder) Embed(text string) ([]float32, error) {
	return nil, errNotImplemented
}

func (e *ONNXEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	return nil, errNotImplemented
}
