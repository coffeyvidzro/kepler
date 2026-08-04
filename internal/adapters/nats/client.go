package nats

import (
	"context"

	legacy "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// Client is the application's NATS JetStream adapter.
type Client = legacy.Client

// New connects to NATS and verifies JetStream availability.
func New(ctx context.Context, url, name string, applications ...*newrelic.Application) (*Client, error) {
	return legacy.New(ctx, url, name, applications...)
}
