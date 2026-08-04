package redis

import "context"

// HealthChecker is implemented by Redis clients that expose Ping.
type HealthChecker interface {
	Ping(context.Context) error
}
