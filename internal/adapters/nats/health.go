package nats

import (
	"context"
	"fmt"
)

// HealthChecker verifies that NATS and JetStream are reachable.
type HealthChecker interface {
	Ping(context.Context) error
}

func (client *Client) Ping(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("JetStream health context is required")
	}
	if client == nil || client.connection == nil || client.jetStream == nil {
		return ErrClientUnavailable
	}
	if !client.connection.IsConnected() {
		return fmt.Errorf("JetStream client is not connected")
	}
	if _, err := client.jetStream.AccountInfo(ctx); err != nil {
		return fmt.Errorf("read JetStream account info: %w", err)
	}
	return nil
}
