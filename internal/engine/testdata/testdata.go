package testdata

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed corpus.json
var corpusJSON []byte

// CorpusEntry is a labeled log line for classification validation.
type CorpusEntry struct {
	Raw              string `json:"raw"`
	ExpectedType     string `json:"expected_type"`
	ExpectedCategory string `json:"expected_category"`
	ExpectedSeverity string `json:"expected_severity"`
	Description      string `json:"description"`
}

// LoadCorpus parses the embedded corpus.json and returns all entries.
func LoadCorpus() ([]CorpusEntry, error) {
	var entries []CorpusEntry
	if err := json.Unmarshal(corpusJSON, &entries); err != nil {
		return nil, fmt.Errorf("parse corpus.json: %w", err)
	}
	return entries, nil
}
