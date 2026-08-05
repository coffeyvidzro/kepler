package arcjet

import (
	"fmt"
	"strings"

	arcjetsdk "github.com/arcjet/arcjet-go"
)

// Client is the configured Arcjet SDK client.
type Client = arcjetsdk.Client

// New creates the application's Arcjet client and default rules.
func New(key string) (*Client, error) {
	key = strings.TrimSpace(key)
	client, err := arcjetsdk.NewClient(arcjetsdk.Config{
		Key:      key,
		Platform: arcjetsdk.PlatformCloudflare,
		Rules: []arcjetsdk.Rule{
			arcjetsdk.Shield(arcjetsdk.ShieldOptions{Mode: arcjetsdk.ModeDryRun}),
			arcjetsdk.DetectBot(arcjetsdk.BotOptions{Mode: arcjetsdk.ModeDryRun, Allow: []string{}}),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create Arcjet client: %w", err)
	}
	return client, nil
}
