package embedder

import "fmt"

// Embedder produces vector embeddings from text.
type Embedder interface {
	Embed(text string) ([]float32, error)
	EmbedBatch(texts []string) ([][]float32, error)
	Close() error
}

// ONNXEmbedder wraps the ONNX runtime, tokenizer, and projection layer for
// local embedding inference.
type ONNXEmbedder struct {
	session *onnxSession
	tok     *tokenizer
	proj    *projection
}

// New creates an ONNXEmbedder by loading the ONNX model, vocabulary, and
// projection weights. The full embedding pipeline is:
// tokenize → ONNX inference → mean pool → dense projection → 1024-dim vector.
func New(modelPath, vocabPath, projectionPath string) (*ONNXEmbedder, error) {
	sess, err := newONNXSession(modelPath)
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}

	tok, err := newTokenizer(vocabPath)
	if err != nil {
		sess.close()
		return nil, fmt.Errorf("embedder: %w", err)
	}

	proj, err := loadProjection(projectionPath)
	if err != nil {
		sess.close()
		return nil, fmt.Errorf("embedder: %w", err)
	}

	if int(sess.embedDim) != proj.inDim {
		sess.close()
		return nil, fmt.Errorf("embedder: ONNX output dim %d != projection input dim %d",
			sess.embedDim, proj.inDim)
	}

	return &ONNXEmbedder{session: sess, tok: tok, proj: proj}, nil
}

// EmbedDim returns the final embedding dimensionality (after projection).
func (e *ONNXEmbedder) EmbedDim() int {
	return e.proj.outDim
}

// Embed produces a single embedding vector for the given text.
// Routes through tokenizeBatch for dynamic padding to actual sequence length.
func (e *ONNXEmbedder) Embed(text string) ([]float32, error) {
	batch := e.tok.tokenizeBatch([]string{text})

	hidden, err := e.session.infer(
		batch.inputIDs, batch.attentionMask, batch.tokenTypeIDs,
		batch.batchSize, batch.seqLen,
	)
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}

	pooled := meanPool(hidden, batch.attentionMask, 1, batch.seqLen, e.session.embedDim)
	return e.proj.apply(pooled), nil
}

// EmbedBatch produces embedding vectors for multiple texts.
func (e *ONNXEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	batch := e.tok.tokenizeBatch(texts)

	hidden, err := e.session.infer(
		batch.inputIDs, batch.attentionMask, batch.tokenTypeIDs,
		batch.batchSize, batch.seqLen,
	)
	if err != nil {
		return nil, fmt.Errorf("embedder: %w", err)
	}

	pooled := meanPool(hidden, batch.attentionMask, batch.batchSize, batch.seqLen, e.session.embedDim)

	dim := e.session.embedDim
	results := make([][]float32, batch.batchSize)
	for i := int64(0); i < batch.batchSize; i++ {
		vec := pooled[i*dim : (i+1)*dim]
		results[i] = e.proj.apply(vec)
	}
	return results, nil
}

// Close releases ONNX Runtime resources.
func (e *ONNXEmbedder) Close() error {
	if e.session != nil {
		return e.session.close()
	}
	return nil
}
