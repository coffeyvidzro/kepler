package security

import (
	"fmt"

	"github.com/arcjet/arcjet-go"
)

func NewClient(arcjetKey string) (*arcjet.Client, error) {
	client, err := arcjet.NewClient(arcjet.Config{
		Key:      arcjetKey,
		Platform: arcjet.PlatformCloudflare,
		Rules: []arcjet.Rule{
			arcjet.Shield(arcjet.ShieldOptions{
				Mode: arcjet.ModeDryRun,
			}),
			arcjet.DetectBot(arcjet.BotOptions{
				Mode:  arcjet.ModeDryRun,
				Allow: []string{}, // observe all detected bots without blocking requests
			}),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create Arcjet client: %w", err)
	}

	return client, nil
}
