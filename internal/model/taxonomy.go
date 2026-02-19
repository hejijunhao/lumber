package model

// TaxonomyNode represents a node in the taxonomy tree.
type TaxonomyNode struct {
	Name     string
	Children []*TaxonomyNode
	Desc     string // description used for embedding
}

// EmbeddedLabel is a taxonomy leaf with its pre-computed embedding vector.
type EmbeddedLabel struct {
	Path   string    // e.g. "ERROR.connection_failure"
	Vector []float32
}
