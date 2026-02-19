package embedder

// meanPool computes attention-mask-weighted mean pooling over the sequence
// dimension of transformer hidden states.
//
// hidden: flat [batchSize * seqLen * dim] float32 (per-token hidden states)
// mask:   flat [batchSize * seqLen] int64 (1 for real tokens, 0 for padding)
//
// Returns flat [batchSize * dim] float32 (one pooled vector per sample).
func meanPool(hidden []float32, mask []int64, batchSize, seqLen, dim int64) []float32 {
	out := make([]float32, batchSize*dim)

	for b := int64(0); b < batchSize; b++ {
		maskOff := b * seqLen
		hiddenOff := b * seqLen * dim
		outOff := b * dim

		// Count non-padding tokens.
		var count float32
		for s := int64(0); s < seqLen; s++ {
			if mask[maskOff+s] == 1 {
				count++
			}
		}
		if count == 0 {
			continue
		}

		// Sum hidden states at non-padding positions.
		for s := int64(0); s < seqLen; s++ {
			if mask[maskOff+s] != 1 {
				continue
			}
			tokOff := hiddenOff + s*dim
			for d := int64(0); d < dim; d++ {
				out[outOff+d] += hidden[tokOff+d]
			}
		}

		// Divide by count.
		inv := 1.0 / count
		for d := int64(0); d < dim; d++ {
			out[outOff+d] *= inv
		}
	}

	return out
}
