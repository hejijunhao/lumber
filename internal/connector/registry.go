package connector

import "fmt"

// Constructor is a function that creates a new Connector instance.
type Constructor func() Connector

var registry = map[string]Constructor{}

// Register adds a connector constructor under the given provider name.
func Register(name string, ctor Constructor) {
	registry[name] = ctor
}

// Get returns the connector constructor for the given provider name.
func Get(name string) (Constructor, error) {
	ctor, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown connector provider: %s", name)
	}
	return ctor, nil
}

// Providers returns the names of all registered connector providers.
func Providers() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
