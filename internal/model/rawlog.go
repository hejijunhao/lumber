package model

import "time"

// RawLog is the intermediate type produced by connectors and consumed by the engine.
type RawLog struct {
	Timestamp time.Time
	Source    string         // provider name (e.g. "vercel", "aws")
	Raw       string         // original log text
	Metadata  map[string]any // provider-specific metadata
}
