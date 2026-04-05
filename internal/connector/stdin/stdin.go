package stdin

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/kaminocorp/lumber/internal/connector"
	"github.com/kaminocorp/lumber/internal/model"
)

// maxLineSize is the maximum line length the scanner will accept (1MB).
// The default bufio.Scanner limit of 64KB is too small for stack traces.
const maxLineSize = 1024 * 1024

func init() {
	connector.Register("stdin", func() connector.Connector {
		return &Connector{reader: os.Stdin}
	})
}

// Connector reads log lines from an io.Reader (defaults to os.Stdin).
type Connector struct {
	reader io.Reader
}

// Option configures a stdin Connector.
type Option func(*Connector)

// WithReader overrides the default os.Stdin reader. Used in tests.
func WithReader(r io.Reader) Option {
	return func(c *Connector) { c.reader = r }
}

// New creates a stdin Connector with the given options.
func New(opts ...Option) *Connector {
	c := &Connector{reader: os.Stdin}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Stream reads lines from the reader and sends each as a RawLog on the
// returned channel. The channel closes on EOF or context cancellation.
func (c *Connector) Stream(ctx context.Context, _ connector.ConnectorConfig) (<-chan model.RawLog, error) {
	ch := make(chan model.RawLog, 64)

	scanner := bufio.NewScanner(c.reader)
	scanner.Buffer(make([]byte, 0, maxLineSize), maxLineSize)

	go func() {
		defer close(ch)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			raw := model.RawLog{
				Timestamp: time.Now(),
				Source:    "stdin",
				Raw:       line,
			}
			select {
			case ch <- raw:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Warn("stdin connector: scanner error", "error", err)
		}
	}()

	return ch, nil
}

// Query is not supported for stdin — it is inherently a streaming source.
func (c *Connector) Query(_ context.Context, _ connector.ConnectorConfig, _ connector.QueryParams) ([]model.RawLog, error) {
	return nil, fmt.Errorf("stdin connector does not support query mode")
}
