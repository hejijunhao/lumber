package compactor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Verbosity controls how much detail is retained after compaction.
type Verbosity int

const (
	Minimal  Verbosity = iota // strip raw logs, minimal metadata
	Standard                  // retain raw logs, moderate metadata
	Full                      // retain everything
)

// defaultStripFields are high-cardinality fields removed from JSON logs at Minimal/Standard.
var defaultStripFields = []string{
	"trace_id", "span_id", "request_id", "x_request_id",
	"correlation_id", "dd.trace_id", "dd.span_id",
}

// Option configures a Compactor.
type Option func(*Compactor)

// WithStripFields overrides the default list of JSON fields to strip.
func WithStripFields(fields []string) Option {
	return func(c *Compactor) {
		c.StripFields = fields
	}
}

// Compactor performs token-aware compaction on log event fields.
type Compactor struct {
	Verbosity   Verbosity
	StripFields []string
}

// New creates a Compactor with the given verbosity level.
func New(v Verbosity, opts ...Option) *Compactor {
	c := &Compactor{
		Verbosity:   v,
		StripFields: defaultStripFields,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Compact applies verbosity-based truncation to the raw log text.
// eventType is the classified event type (e.g. "ERROR") used for type-aware logic.
// Returns the compacted text and a one-line summary.
func (c *Compactor) Compact(raw, eventType string) (compacted string, summary string) {
	result := raw

	// Strip high-cardinality JSON fields at Minimal/Standard.
	if c.Verbosity != Full {
		result = stripFields(result, c.StripFields)
	}

	// Apply truncation.
	switch c.Verbosity {
	case Minimal:
		if eventType == "ERROR" {
			if t := truncateStackTrace(result, 5); t != result {
				result = t
			} else {
				result = truncate(result, 200)
			}
		} else {
			result = truncate(result, 200)
		}
	case Standard:
		if eventType == "ERROR" {
			if t := truncateStackTrace(result, 10); t != result {
				result = t
			} else {
				result = truncate(result, 2000)
			}
		} else {
			result = truncate(result, 2000)
		}
	case Full:
		// preserve everything
	}

	return result, summarize(raw)
}

// truncate cuts the string at maxRunes rune boundary, appending "..." if truncated.
func truncate(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i] + "..."
		}
		count++
	}
	return s
}

// summarize extracts the first line, trimmed to 120 runes at a word boundary.
func summarize(raw string) string {
	// Extract first line.
	line := raw
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		line = raw[:idx]
	}
	line = strings.TrimSpace(line)

	if utf8.RuneCountInString(line) <= 120 {
		return line
	}

	// Find a word boundary at or before 120 runes.
	count := 0
	cutByte := 0
	for i := range line {
		if count == 120 {
			cutByte = i
			break
		}
		count++
	}

	// Walk back to find last space.
	lastSpace := strings.LastIndex(line[:cutByte], " ")
	if lastSpace > 0 {
		return line[:lastSpace] + "..."
	}
	// No space found — cut at rune boundary.
	return line[:cutByte] + "..."
}

// Stack trace frame patterns.
var (
	javaFrameRe = regexp.MustCompile(`^\s+at `)
	goFrameRe   = regexp.MustCompile(`^\s+.+\.go:\d+`)
	goRoutineRe = regexp.MustCompile(`^goroutine \d+`)
)

// truncateStackTrace detects stack trace patterns and preserves first maxFrames
// and last 2 frames, replacing the middle with an omission message.
// Uses range-based cut: everything between the last kept first-frame and the
// first kept last-frame is replaced wholesale, including interleaved non-frame
// lines (e.g. Go function signatures paired with source locations).
func truncateStackTrace(raw string, maxFrames int) string {
	lines := strings.Split(raw, "\n")

	// Find frame line indices.
	var frames []int
	for i, line := range lines {
		if javaFrameRe.MatchString(line) || goFrameRe.MatchString(line) || goRoutineRe.MatchString(line) {
			frames = append(frames, i)
		}
	}

	// Not enough frames to truncate.
	const tailFrames = 2
	if len(frames) <= maxFrames+tailFrames {
		return raw
	}

	// Range cut: keep lines[0..lastKeptFirst] and lines[firstKeptLast..end].
	lastKeptFirst := frames[maxFrames-1]
	firstKeptLast := frames[len(frames)-tailFrames]
	omitted := len(frames) - maxFrames - tailFrames
	omissionMsg := fmt.Sprintf("\t... (%d frames omitted) ...", omitted)

	var result []string
	result = append(result, lines[:lastKeptFirst+1]...)
	result = append(result, omissionMsg)
	result = append(result, lines[firstKeptLast:]...)

	return strings.Join(result, "\n")
}

// stripFields removes high-cardinality keys from JSON-formatted log lines.
// Non-JSON lines pass through unchanged.
func stripFields(raw string, fields []string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return raw
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(trimmed), &m); err != nil {
		return raw
	}

	changed := false
	for _, f := range fields {
		if _, ok := m[f]; ok {
			delete(m, f)
			changed = true
		}
	}
	if !changed {
		return raw
	}

	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return string(out)
}
