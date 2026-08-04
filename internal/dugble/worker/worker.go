package worker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	workerruntime "github.com/coffeyvidzro/dugble/server/internal/worker"
)

// Worker owns the supervised background components and their process lifecycle.
type Worker struct {
	supervisor    *workerruntime.Supervisor
	healthAddress string
	metricsPath   string
}

// New creates a worker from an initialized supervisor.
func New(supervisor *workerruntime.Supervisor, healthAddress, metricsPath string) (*Worker, error) {
	if supervisor == nil {
		return nil, errors.New("worker supervisor is required")
	}
	return &Worker{
		supervisor:    supervisor,
		healthAddress: strings.TrimSpace(healthAddress),
		metricsPath:   strings.TrimSpace(metricsPath),
	}, nil
}

// Run starts every supervised component and blocks until shutdown.
func (worker *Worker) Run(ctx context.Context) error {
	if worker == nil || worker.supervisor == nil {
		return errors.New("worker is not configured")
	}
	if ctx == nil {
		return errors.New("worker context is required")
	}

	slog.Info(
		"worker starting",
		"failure_policy", worker.supervisor.Policy(),
		"health_address", worker.healthAddress,
		"metrics_path", worker.metricsPath,
	)
	if err := worker.supervisor.Run(ctx, 30*time.Second); err != nil {
		return err
	}
	slog.Info("worker stopped")
	return nil
}
