package postgres

import "context"

// HealthChecker is implemented by PostgreSQL pools and connections.
type HealthChecker interface {
	Ping(context.Context) error
}

// Ping checks PostgreSQL connectivity through checker.
func Ping(ctx context.Context, checker HealthChecker) error {
	return checker.Ping(ctx)
}
