package nats

import "context"

// HealthChecker verifies that NATS and JetStream are reachable.
type HealthChecker interface {
	Ping(context.Context) error
}
