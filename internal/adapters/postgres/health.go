package postgres

import (
	"context"
	"errors"
	"fmt"
)

// HealthChecker is implemented by PostgreSQL pools and connections.
type HealthChecker interface {
	Ping(context.Context) error
}

func Ping(ctx context.Context, checker HealthChecker) error {
	if ctx == nil {
		return errors.New("PostgreSQL health context is required")
	}
	if checker == nil {
		return errors.New("PostgreSQL health checker is required")
	}
	if err := checker.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}
