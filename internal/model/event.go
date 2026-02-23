package model

import "time"

// CanonicalEvent is Lumber's output type — a classified, normalized log event.
type CanonicalEvent struct {
	Type       string    `json:"type"`
	Category   string    `json:"category"`
	Severity   string    `json:"severity"`
	Timestamp  time.Time `json:"timestamp"`
	Summary    string    `json:"summary"`
	Confidence float64   `json:"confidence,omitempty"`
	Raw        string    `json:"raw,omitempty"`
	Count      int       `json:"count,omitempty"` // >0 when deduplicated
}
