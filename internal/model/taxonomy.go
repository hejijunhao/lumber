package model

// TaxonomyNode represents a node in the taxonomy tree.
type TaxonomyNode struct {
	Name     string
	Children []*TaxonomyNode
	Desc     string // description used for embedding
	Severity string // leaf-level severity (error, warning, info, debug)
}

// EmbeddedLabel is a taxonomy leaf with its pre-computed embedding vector.
type EmbeddedLabel struct {
	Path     string    // e.g. "ERROR.connection_failure"
	Vector   []float32
	Severity string // leaf-level severity carried from TaxonomyNode
}
