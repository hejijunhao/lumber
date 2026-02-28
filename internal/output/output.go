package output

import (
	"context"

	"github.com/hejijunhao/lumber/internal/model"
)

// Output defines the interface for canonical event destinations.
type Output interface {
	Write(ctx context.Context, event model.CanonicalEvent) error
	Close() error
}
