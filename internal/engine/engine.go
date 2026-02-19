package engine

import (
	"strings"

	"github.com/crimson-sun/lumber/internal/engine/classifier"
	"github.com/crimson-sun/lumber/internal/engine/compactor"
	"github.com/crimson-sun/lumber/internal/engine/embedder"
	"github.com/crimson-sun/lumber/internal/engine/taxonomy"
	"github.com/crimson-sun/lumber/internal/model"
)

// Engine orchestrates the embed → classify → compact pipeline.
type Engine struct {
	embedder   embedder.Embedder
	taxonomy   *taxonomy.Taxonomy
	classifier *classifier.Classifier
	compactor  *compactor.Compactor
}

// New creates an Engine with the provided components.
func New(emb embedder.Embedder, tax *taxonomy.Taxonomy, cls *classifier.Classifier, cmp *compactor.Compactor) *Engine {
	return &Engine{
		embedder:   emb,
		taxonomy:   tax,
		classifier: cls,
		compactor:  cmp,
	}
}

// Process classifies and compacts a single raw log into a canonical event.
func (e *Engine) Process(raw model.RawLog) (model.CanonicalEvent, error) {
	vec, err := e.embedder.Embed(raw.Raw)
	if err != nil {
		return model.CanonicalEvent{}, err
	}

	result := e.classifier.Classify(vec, e.taxonomy.Labels())

	compacted, summary := e.compactor.Compact(raw.Raw)

	parts := strings.SplitN(result.Label.Path, ".", 2)
	eventType := parts[0]
	category := ""
	if len(parts) > 1 {
		category = parts[1]
	}

	return model.CanonicalEvent{
		Type:       eventType,
		Category:   category,
		Severity:   inferSeverity(eventType),
		Timestamp:  raw.Timestamp,
		Summary:    summary,
		Confidence: result.Confidence,
		Raw:        compacted,
	}, nil
}

// ProcessBatch classifies and compacts a slice of raw logs.
func (e *Engine) ProcessBatch(raws []model.RawLog) ([]model.CanonicalEvent, error) {
	events := make([]model.CanonicalEvent, 0, len(raws))
	for _, raw := range raws {
		ev, err := e.Process(raw)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

func inferSeverity(eventType string) string {
	switch eventType {
	case "ERROR":
		return "error"
	case "SECURITY":
		return "warning"
	case "DEPLOY":
		return "info"
	default:
		return "info"
	}
}
