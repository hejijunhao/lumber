package stdout

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/crimson-sun/lumber/internal/model"
)

// Output writes JSON-encoded canonical events to stdout.
type Output struct {
	enc *json.Encoder
}

// New creates a new stdout Output.
func New() *Output {
	return &Output{enc: json.NewEncoder(os.Stdout)}
}

func (o *Output) Write(_ context.Context, event model.CanonicalEvent) error {
	if err := o.enc.Encode(event); err != nil {
		return fmt.Errorf("stdout output: %w", err)
	}
	return nil
}

func (o *Output) Close() error {
	return nil
}
