package model

import "time"

// CanonicalEvent is Lumber's output type — a classified, normalized log event.
type CanonicalEvent struct {
	Type       string    // top-level category (ERROR, REQUEST, DEPLOY, etc.)
	Category   string    // leaf label (connection_failure, build_succeeded, etc.)
	Severity   string    // normalized severity
	Timestamp  time.Time
	Summary    string    // human-readable summary
	Confidence float64   // classification confidence score
	Raw        string    // original log text (retained at standard/full verbosity)
}
