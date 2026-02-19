package taxonomy

import (
	"github.com/crimson-sun/lumber/internal/engine/embedder"
	"github.com/crimson-sun/lumber/internal/model"
)

// Taxonomy manages the label tree and pre-embedded label vectors.
type Taxonomy struct {
	root   []*model.TaxonomyNode
	labels []model.EmbeddedLabel
}

// New creates a Taxonomy from a set of root nodes and pre-embeds all leaf labels.
func New(roots []*model.TaxonomyNode, emb embedder.Embedder) (*Taxonomy, error) {
	t := &Taxonomy{root: roots}
	// Pre-embedding will happen here once the embedder is implemented.
	_ = emb
	return t, nil
}

// Labels returns the pre-embedded taxonomy labels for classification.
func (t *Taxonomy) Labels() []model.EmbeddedLabel {
	return t.labels
}

// Roots returns the top-level taxonomy nodes.
func (t *Taxonomy) Roots() []*model.TaxonomyNode {
	return t.root
}
