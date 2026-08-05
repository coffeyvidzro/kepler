package nats

import (
	"context"
	"fmt"
	"strings"

	natsjs "github.com/nats-io/nats.go/jetstream"
)

// ConsumerManager creates or updates durable JetStream consumers.
type ConsumerManager interface {
	CreateOrUpdateConsumer(context.Context, string, natsjs.ConsumerConfig) (natsjs.Consumer, error)
}

func (client *Client) CreateOrUpdateConsumer(
	ctx context.Context,
	stream string,
	config natsjs.ConsumerConfig,
) (natsjs.Consumer, error) {
	if ctx == nil {
		return nil, fmt.Errorf("JetStream consumer context is required")
	}
	if client == nil || client.jetStream == nil {
		return nil, ErrClientUnavailable
	}
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return nil, fmt.Errorf("JetStream stream name is required")
	}
	if strings.TrimSpace(config.Durable) == "" && strings.TrimSpace(config.Name) == "" {
		return nil, fmt.Errorf("JetStream consumer durable or name is required")
	}
	consumer, err := client.jetStream.CreateOrUpdateConsumer(ctx, stream, config)
	if err != nil {
		return nil, fmt.Errorf(
			"create or update consumer %q on %s: %w",
			consumerName(config),
			stream,
			err,
		)
	}
	return consumer, nil
}

func consumerName(config natsjs.ConsumerConfig) string {
	if durable := strings.TrimSpace(config.Durable); durable != "" {
		return durable
	}
	return strings.TrimSpace(config.Name)
}
