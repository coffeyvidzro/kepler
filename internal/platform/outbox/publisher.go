package outbox

import "context"

// Publisher publishes an outbox message to the message bus.
type Publisher interface {
	Publish(context.Context, string, []byte, map[string]string, string) error
}
