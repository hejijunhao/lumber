package compactor

// Verbosity controls how much detail is retained after compaction.
type Verbosity int

const (
	Minimal  Verbosity = iota // strip raw logs, minimal metadata
	Standard                  // retain raw logs, moderate metadata
	Full                      // retain everything
)

// Compactor performs token-aware compaction on log event fields.
type Compactor struct {
	Verbosity Verbosity
}

// New creates a Compactor with the given verbosity level.
func New(v Verbosity) *Compactor {
	return &Compactor{Verbosity: v}
}

// Compact applies verbosity-based truncation and deduplication to the raw log text.
// Returns the compacted text and summary.
func (c *Compactor) Compact(raw string) (compacted string, summary string) {
	switch c.Verbosity {
	case Minimal:
		return truncate(raw, 200), summarize(raw)
	case Standard:
		return truncate(raw, 2000), summarize(raw)
	case Full:
		return raw, summarize(raw)
	default:
		return raw, summarize(raw)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func summarize(raw string) string {
	if len(raw) <= 120 {
		return raw
	}
	return raw[:120] + "..."
}
