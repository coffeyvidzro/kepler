package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	db     *pgxpool.Pool
	config Config
	now    func() time.Time
}

func NewWorker(db *pgxpool.Pool, config Config) *Worker {
	return &Worker{
		db:     db,
		config: normalizeConfig(config),
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (worker *Worker) Run(ctx context.Context) error {
	if worker == nil || worker.db == nil {
		return ErrWorkerNotConfigured
	}
	ticker := time.NewTicker(worker.config.PollInterval)
	defer ticker.Stop()
	for {
		deleted, err := worker.deleteBatch(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.ErrorContext(ctx, "verification cleanup failed", "error", err)
		} else if deleted > 0 {
			slog.InfoContext(ctx, "verification cleanup completed", "deleted", deleted)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (worker *Worker) deleteBatch(ctx context.Context) (int64, error) {
	if worker == nil || worker.db == nil {
		return 0, ErrWorkerNotConfigured
	}
	batchCtx, cancel := context.WithTimeout(ctx, worker.config.BatchTimeout)
	defer cancel()
	cutoff := worker.now().Add(-worker.config.Retention)
	result, err := worker.db.Exec(batchCtx, `
		WITH candidates AS (
			SELECT id
			FROM verifications
			WHERE status IN ('approved', 'expired', 'canceled', 'max_attempts_reached', 'delivery_failed')
			  AND updated_at < $1
			ORDER BY updated_at, id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM verifications AS verification
		USING candidates
		WHERE verification.id = candidates.id
	`, cutoff, worker.config.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("delete retained verifications: %w", err)
	}
	return result.RowsAffected(), nil
}
