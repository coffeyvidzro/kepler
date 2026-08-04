package nats

import (
	"context"

	natsjs "github.com/nats-io/nats.go/jetstream"
)

// ConsumerManager creates or updates durable JetStream consumers.
type ConsumerManager interface {
	CreateOrUpdateConsumer(context.Context, string, natsjs.ConsumerConfig) (natsjs.Consumer, error)
}
