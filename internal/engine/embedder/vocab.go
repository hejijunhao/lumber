package embedder

import (
	"bufio"
	"fmt"
	"os"
)

// vocab holds a WordPiece vocabulary loaded from a vocab.txt file.
// Token IDs are determined by line number (0-indexed).
type vocab struct {
	tokenToID map[string]int64
	idToToken []string

	padID int64
	unkID int64
	clsID int64
	sepID int64
}

// loadVocab reads a vocab.txt file where each line is a token and the line
// number (0-indexed) is the token ID.
func loadVocab(path string) (*vocab, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("vocab: %w", err)
	}
	defer f.Close()

	var tokens []string
	tokenToID := make(map[string]int64, 32000)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		tok := scanner.Text()
		id := int64(len(tokens))
		tokenToID[tok] = id
		tokens = append(tokens, tok)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("vocab: read error: %w", err)
	}
	if len(tokens) == 0 {
		return nil, fmt.Errorf("vocab: file is empty: %s", path)
	}

	v := &vocab{
		tokenToID: tokenToID,
		idToToken: tokens,
	}

	// Resolve special token IDs.
	specials := []struct {
		name string
		dest *int64
	}{
		{"[PAD]", &v.padID},
		{"[UNK]", &v.unkID},
		{"[CLS]", &v.clsID},
		{"[SEP]", &v.sepID},
	}
	for _, s := range specials {
		id, ok := tokenToID[s.name]
		if !ok {
			return nil, fmt.Errorf("vocab: missing special token %s", s.name)
		}
		*s.dest = id
	}

	return v, nil
}

// lookup returns the token ID for the given token, or the [UNK] ID if not found.
func (v *vocab) lookup(token string) int64 {
	if id, ok := v.tokenToID[token]; ok {
		return id
	}
	return v.unkID
}

// contains reports whether the token is in the vocabulary.
func (v *vocab) contains(token string) bool {
	_, ok := v.tokenToID[token]
	return ok
}

// size returns the number of tokens in the vocabulary.
func (v *vocab) size() int {
	return len(v.idToToken)
}
