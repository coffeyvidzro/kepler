package nats

import (
	legacy "github.com/coffeyvidzro/dugble/server/internal/messaging/jetstream"
	natsjs "github.com/nats-io/nats.go/jetstream"
)

// StreamLimits controls storage and retention for application streams.
type StreamLimits = legacy.StreamLimits

// DefaultStreamLimits returns the production stream defaults.
func DefaultStreamLimits() StreamLimits {
	return legacy.DefaultStreamLimits()
}

// StreamConfigs builds the desired JetStream stream configuration.
func StreamConfigs(limits StreamLimits) []natsjs.StreamConfig {
	return legacy.StreamConfigs(limits)
}
