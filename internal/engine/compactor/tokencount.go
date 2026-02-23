package compactor

import (
	"math"
	"strings"
)

// EstimateTokens returns an approximate token count using a whitespace heuristic.
// Splits on whitespace, applies a 1.3x subword expansion factor (rounded up).
// Not a real tokenizer — accurate within ~20% of BPE counts, sufficient for
// measuring compaction ratios.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	words := len(strings.Fields(s))
	return int(math.Ceil(float64(words) * 1.3))
}
