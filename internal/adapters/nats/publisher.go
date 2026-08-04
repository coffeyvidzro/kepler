package nats

import "context"

// Publisher publishes a deduplicated message to JetStream.
type Publisher interface {
	Publish(context.Context, string, []byte, map[string]string, string) error
}
