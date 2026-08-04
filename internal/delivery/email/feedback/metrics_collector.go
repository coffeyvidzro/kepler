package feedback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MetricsCollector struct {
	db       *pgxpool.Pool
	metrics  *Metrics
	interval time.Duration
}

func NewMetricsCollector(db *pgxpool.Pool, metrics *Metrics, interval time.Duration) *MetricsCollector {
	if metrics == nil {
		metrics = DefaultMetrics
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	return &MetricsCollector{db: db, metrics: metrics, interval: interval}
}

func (c *MetricsCollector) Run(ctx context.Context) error {
	if c == nil || c.db == nil || c.metrics == nil {
		return errors.New("email feedback metrics collector is not configured")
	}
	if err := c.collect(ctx); err != nil && ctx.Err() == nil {
		return err
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := c.collect(ctx); err != nil && ctx.Err() == nil {
				return err
			}
		}
	}
}

func (c *MetricsCollector) collect(ctx context.Context) error {
	startedAt := time.Now()
	var snapshot QueueSnapshot
	err := c.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (
				WHERE processed_at IS NULL
				  AND dead_lettered_at IS NULL
				  AND next_attempt_at IS NOT NULL
				  AND next_attempt_at <= now()
			),
			count(*) FILTER (
				WHERE processed_at IS NULL
				  AND dead_lettered_at IS NULL
				  AND next_attempt_at IS NOT NULL
			),
			count(*) FILTER (WHERE dead_lettered_at IS NOT NULL),
			count(*) FILTER (
				WHERE processed_at IS NULL
				  AND dead_lettered_at IS NULL
				  AND email_message_id IS NULL
			)
		FROM email_provider_events
	`).Scan(&snapshot.Due, &snapshot.Scheduled, &snapshot.DeadLettered, &snapshot.Unlinked)
	c.metrics.ObserveOperation("queue_snapshot", time.Since(startedAt))
	if err != nil {
		c.metrics.RecordEvent("metrics", "queue", "error")
		return fmt.Errorf("collect email feedback reconciliation metrics: %w", err)
	}
	c.metrics.SetReconciliationQueue(snapshot)
	c.metrics.RecordEvent("metrics", "queue", "success")
	return nil
}
