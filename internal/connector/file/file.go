package file

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kaminocorp/lumber/internal/connector"
	"github.com/kaminocorp/lumber/internal/model"
)

// maxLineSize is the maximum line length the scanner will accept (1MB).
const maxLineSize = 1024 * 1024

func init() {
	connector.Register("file", func() connector.Connector {
		return &Connector{}
	})
}

// Connector reads log lines from a file on disk.
type Connector struct{}

// Stream reads all lines from the file specified in cfg.Extra["file"]
// and sends each as a RawLog on the returned channel. The channel closes
// on EOF or context cancellation.
func (c *Connector) Stream(ctx context.Context, cfg connector.ConnectorConfig) (<-chan model.RawLog, error) {
	filePath, err := resolveFilePath(cfg)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("file connector: %w", err)
	}

	ch := make(chan model.RawLog, 64)
	go func() {
		defer close(ch)
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, maxLineSize), maxLineSize)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			raw := model.RawLog{
				Timestamp: time.Now(),
				Source:    "file",
				Raw:       line,
				Metadata: map[string]any{
					"file": filepath.Base(filePath),
				},
			}
			select {
			case ch <- raw:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			slog.Warn("file connector: scanner error", "error", err, "file", filePath)
		}
	}()

	return ch, nil
}

// Query reads lines from the file, returning up to params.Limit results.
// Start/End time filters are not applicable to file lines (they have no
// inherent timestamp) and are ignored.
func (c *Connector) Query(_ context.Context, cfg connector.ConnectorConfig, params connector.QueryParams) ([]model.RawLog, error) {
	filePath, err := resolveFilePath(cfg)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("file connector: %w", err)
	}
	defer f.Close()

	if !params.Start.IsZero() || !params.End.IsZero() {
		slog.Debug("file connector: time range filters ignored (file lines have no inherent timestamp)")
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, maxLineSize), maxLineSize)

	var results []model.RawLog
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		results = append(results, model.RawLog{
			Timestamp: time.Now(),
			Source:    "file",
			Raw:       line,
			Metadata: map[string]any{
				"file": filepath.Base(filePath),
			},
		})
		if params.Limit > 0 && len(results) >= params.Limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return results, fmt.Errorf("file connector: scanner error: %w", err)
	}

	return results, nil
}

// resolveFilePath extracts and validates the file path from connector config.
func resolveFilePath(cfg connector.ConnectorConfig) (string, error) {
	filePath := cfg.Extra["file"]
	if filePath == "" {
		return "", fmt.Errorf("file connector: missing required config key \"file\" in Extra (set via -file flag or LUMBER_FILE_PATH)")
	}
	return filePath, nil
}
