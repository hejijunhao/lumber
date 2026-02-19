package embedder

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const maxSeqLen = 128

// tokenized holds the result of tokenizing one or more texts, ready for ONNX
// inference. All slices are flat: [batchSize * seqLen].
type tokenized struct {
	inputIDs      []int64
	attentionMask []int64
	tokenTypeIDs  []int64
	batchSize     int64
	seqLen        int64
}

// tokenizer performs BERT-style WordPiece tokenization.
type tokenizer struct {
	vocab *vocab
}

// newTokenizer creates a tokenizer from a vocab.txt file.
func newTokenizer(vocabPath string) (*tokenizer, error) {
	v, err := loadVocab(vocabPath)
	if err != nil {
		return nil, err
	}
	return &tokenizer{vocab: v}, nil
}

// tokenize converts a single text into token IDs with [CLS] and [SEP],
// truncated to maxSeqLen. The returned slices have length maxSeqLen (padded).
func (t *tokenizer) tokenize(text string) (inputIDs, attentionMask, tokenTypeIDs []int64) {
	tokens := t.wordpiece(t.basicTokenize(text))

	// Truncate to fit [CLS] + tokens + [SEP] within maxSeqLen.
	maxTokens := maxSeqLen - 2
	if len(tokens) > maxTokens {
		tokens = tokens[:maxTokens]
	}

	// Build ID sequence: [CLS] tokens... [SEP] [PAD]...
	ids := make([]int64, maxSeqLen)
	mask := make([]int64, maxSeqLen)
	typeIDs := make([]int64, maxSeqLen) // all zeros

	ids[0] = t.vocab.clsID
	mask[0] = 1
	for i, tok := range tokens {
		ids[i+1] = t.vocab.lookup(tok)
		mask[i+1] = 1
	}
	ids[len(tokens)+1] = t.vocab.sepID
	mask[len(tokens)+1] = 1
	// Remaining positions stay 0 (padID=0, mask=0, typeIDs=0).

	return ids, mask, typeIDs
}

// tokenizeBatch tokenizes multiple texts and packs them into flat slices
// padded to the longest sequence in the batch (capped at maxSeqLen).
func (t *tokenizer) tokenizeBatch(texts []string) tokenized {
	n := len(texts)
	if n == 0 {
		return tokenized{}
	}

	// Tokenize each text individually to find per-sequence lengths.
	type seq struct {
		ids  []int64
		mask []int64
		len  int // number of real tokens (non-padding)
	}
	seqs := make([]seq, n)
	maxLen := int64(0)

	for i, text := range texts {
		ids, mask, _ := t.tokenize(text)
		// Count real tokens.
		realLen := 0
		for _, m := range mask {
			if m == 1 {
				realLen++
			}
		}
		seqs[i] = seq{ids: ids, mask: mask, len: realLen}
		if int64(realLen) > maxLen {
			maxLen = int64(realLen)
		}
	}

	// Pack into flat slices, trimmed to maxLen (the longest sequence).
	batchSize := int64(n)
	seqLen := maxLen
	total := batchSize * seqLen

	inputIDs := make([]int64, total)
	attentionMask := make([]int64, total)
	tokenTypeIDs := make([]int64, total) // all zeros

	for i, s := range seqs {
		offset := int64(i) * seqLen
		copy(inputIDs[offset:offset+seqLen], s.ids[:seqLen])
		copy(attentionMask[offset:offset+seqLen], s.mask[:seqLen])
	}

	return tokenized{
		inputIDs:      inputIDs,
		attentionMask: attentionMask,
		tokenTypeIDs:  tokenTypeIDs,
		batchSize:     batchSize,
		seqLen:        seqLen,
	}
}

// basicTokenize applies BERT's BasicTokenizer: clean, lowercase, strip
// accents, split on whitespace and punctuation, handle CJK characters.
func (t *tokenizer) basicTokenize(text string) []string {
	text = cleanText(text)
	text = tokenizeChineseChars(text)
	text = strings.ToLower(text)
	text = stripAccents(text)

	// Split on whitespace, then split each token on punctuation.
	var tokens []string
	for _, word := range strings.Fields(text) {
		tokens = append(tokens, splitOnPunctuation(word)...)
	}
	return tokens
}

// wordpiece applies the WordPiece algorithm to a list of basic tokens.
func (t *tokenizer) wordpiece(tokens []string) []string {
	var result []string
	for _, token := range tokens {
		if len(token) == 0 {
			continue
		}
		// If the whole token is unknown and longer than max subword length,
		// we still try to decompose it.
		subTokens := t.wordpieceToken(token)
		result = append(result, subTokens...)
	}
	return result
}

// wordpieceToken decomposes a single basic token into WordPiece subwords.
func (t *tokenizer) wordpieceToken(token string) []string {
	runes := []rune(token)
	if len(runes) > 200 {
		return []string{"[UNK]"}
	}

	var subTokens []string
	start := 0
	for start < len(runes) {
		end := len(runes)
		found := false
		for end > start {
			sub := string(runes[start:end])
			if start > 0 {
				sub = "##" + sub
			}
			if t.vocab.contains(sub) {
				subTokens = append(subTokens, sub)
				found = true
				break
			}
			end--
		}
		if !found {
			return []string{"[UNK]"}
		}
		start = end
	}
	return subTokens
}

// cleanText removes control characters and replaces whitespace with spaces.
func cleanText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		if r == 0 || r == 0xFFFD || isControl(r) {
			continue
		}
		if isWhitespace(r) {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// stripAccents removes combining diacritical marks after NFD normalization.
func stripAccents(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range norm.NFD.String(text) {
		if unicode.In(r, unicode.Mn) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// tokenizeChineseChars adds spaces around CJK Unified Ideographs so they
// become individual tokens.
func tokenizeChineseChars(text string) string {
	var b strings.Builder
	b.Grow(len(text) + len(text)/4)
	for _, r := range text {
		if isChineseChar(r) {
			b.WriteRune(' ')
			b.WriteRune(r)
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitOnPunctuation splits a word at each punctuation character, keeping
// the punctuation as separate tokens.
func splitOnPunctuation(word string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range word {
		if isPunctuation(r) {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			tokens = append(tokens, string(r))
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// Character classification helpers — these match BERT's Python implementation.

func isWhitespace(r rune) bool {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return unicode.Is(unicode.Zs, r)
}

func isControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return unicode.IsControl(r)
}

func isPunctuation(r rune) bool {
	// BERT treats anything in ASCII range 33-47, 58-64, 91-96, 123-126 as
	// punctuation, plus Unicode punctuation categories.
	if (r >= 33 && r <= 47) || (r >= 58 && r <= 64) ||
		(r >= 91 && r <= 96) || (r >= 123 && r <= 126) {
		return true
	}
	return unicode.IsPunct(r)
}

func isChineseChar(r rune) bool {
	// CJK Unified Ideographs and extension ranges.
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3400 && r <= 0x4DBF) ||
		(r >= 0x20000 && r <= 0x2A6DF) ||
		(r >= 0x2A700 && r <= 0x2B73F) ||
		(r >= 0x2B740 && r <= 0x2B81F) ||
		(r >= 0x2B820 && r <= 0x2CEAF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0x2F800 && r <= 0x2FA1F)
}
