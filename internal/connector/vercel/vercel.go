package vercel

import (
	"context"
	"errors"

	"github.com/crimson-sun/lumber/internal/connector"
	"github.com/crimson-sun/lumber/internal/model"
)

var errNotImplemented = errors.New("vercel connector: not implemented")

func init() {
	connector.Register("vercel", func() connector.Connector {
		return &Connector{}
	})
}

// Connector implements the connector.Connector interface for Vercel's log drain / REST API.
type Connector struct{}

func (c *Connector) Stream(ctx context.Context, cfg connector.ConnectorConfig) (<-chan model.RawLog, error) {
	return nil, errNotImplemented
}

func (c *Connector) Query(ctx context.Context, cfg connector.ConnectorConfig, params connector.QueryParams) ([]model.RawLog, error) {
	return nil, errNotImplemented
}
