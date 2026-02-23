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

	parts := strings.SplitN(result.Label.Path, ".", 2)
	eventType := parts[0]
	category := ""
	if len(parts) > 1 {
		category = parts[1]
	}

	compacted, summary := e.compactor.Compact(raw.Raw, eventType)

	return model.CanonicalEvent{
		Type:       eventType,
		Category:   category,
		Severity:   result.Label.Severity,
		Timestamp:  raw.Timestamp,
		Summary:    summary,
		Confidence: result.Confidence,
		Raw:        compacted,
	}, nil
}

// ProcessBatch classifies and compacts a slice of raw logs using a single
// batched ONNX inference call.
func (e *Engine) ProcessBatch(raws []model.RawLog) ([]model.CanonicalEvent, error) {
	if len(raws) == 0 {
		return nil, nil
	}

	texts := make([]string, len(raws))
	for i, raw := range raws {
		texts[i] = raw.Raw
	}

	vecs, err := e.embedder.EmbedBatch(texts)
	if err != nil {
		return nil, err
	}

	events := make([]model.CanonicalEvent, len(raws))
	for i, raw := range raws {
		result := e.classifier.Classify(vecs[i], e.taxonomy.Labels())

		parts := strings.SplitN(result.Label.Path, ".", 2)
		eventType := parts[0]
		category := ""
		if len(parts) > 1 {
			category = parts[1]
		}

		compacted, summary := e.compactor.Compact(raw.Raw, eventType)

		events[i] = model.CanonicalEvent{
			Type:       eventType,
			Category:   category,
			Severity:   result.Label.Severity,
			Timestamp:  raw.Timestamp,
			Summary:    summary,
			Confidence: result.Confidence,
			Raw:        compacted,
		}
	}
	return events, nil
}
