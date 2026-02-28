package lumber

import "time"

// Event is a classified, normalized log event.
// This is the stable public type — internal representations may evolve
// independently without breaking consumers.
type Event struct {
	Type       string    `json:"type"`               // Root category: ERROR, REQUEST, DEPLOY, etc.
	Category   string    `json:"category"`            // Leaf label: connection_failure, success, etc.
	Severity   string    `json:"severity"`            // error, warning, info, debug
	Timestamp  time.Time `json:"timestamp"`           // When the log was produced
	Summary    string    `json:"summary"`             // First line, <=120 runes
	Confidence float64   `json:"confidence,omitempty"` // Cosine similarity score
	Raw        string    `json:"raw,omitempty"`        // Compacted original text
	Count      int       `json:"count,omitempty"`      // >0 when deduplicated
}
