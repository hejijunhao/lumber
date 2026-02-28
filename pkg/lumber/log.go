package lumber

import "time"

// Log is a raw log entry with optional metadata. Use with ClassifyLog
// when you have timestamp and source information.
// For raw text strings, use Classify() instead.
type Log struct {
	Text      string         // The log text to classify
	Timestamp time.Time      // When the log was produced (zero = time.Now())
	Source    string         // Provider/origin name (optional)
	Metadata  map[string]any // Additional context (optional, not used in classification)
}
