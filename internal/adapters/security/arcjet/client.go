package arcjet

import (
	legacy "github.com/coffeyvidzro/dugble/server/internal/integration/security"
	arcjetsdk "github.com/arcjet/arcjet-go"
)

// Client is the configured Arcjet SDK client.
type Client = arcjetsdk.Client

// New creates the application's Arcjet client and default rules.
func New(key string) (*Client, error) {
	return legacy.NewClient(key)
}
