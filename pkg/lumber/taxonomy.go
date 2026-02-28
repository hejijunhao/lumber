package lumber

// Category represents a taxonomy root category with its leaf labels.
type Category struct {
	Name   string  // Root name: ERROR, REQUEST, DEPLOY, etc.
	Labels []Label // Leaf labels under this root
}

// Label represents a single taxonomy leaf.
type Label struct {
	Name     string // e.g. "connection_failure"
	Path     string // e.g. "ERROR.connection_failure"
	Severity string // error, warning, info, debug
}

// Taxonomy returns the current taxonomy tree. This is read-only —
// consumers can inspect available categories but not modify them.
func (l *Lumber) Taxonomy() []Category {
	roots := l.taxonomy.Roots()
	categories := make([]Category, len(roots))
	for i, root := range roots {
		labels := make([]Label, len(root.Children))
		for j, child := range root.Children {
			labels[j] = Label{
				Name:     child.Name,
				Path:     root.Name + "." + child.Name,
				Severity: child.Severity,
			}
		}
		categories[i] = Category{
			Name:   root.Name,
			Labels: labels,
		}
	}
	return categories
}
