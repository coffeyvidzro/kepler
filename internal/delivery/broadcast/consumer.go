package broadcastexecution

import (
	"context"
	"errors"
	"log/slog"
	"time"

	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
)

type repository interface {
	QueueNextDueScheduled(context.Context) (broadcastmodule.Broadcast, bool, error)
	MaterializeNextQueuedRecipients(context.Context) (broadcastmodule.MaterializationResult, bool, error)
}

type Config struct {
	PollInterval time.Duration
	BatchSize    int
}

type Consumer struct {
	repository repository
	config     Config
}

func NewConsumer(repository repository, config Config) *Consumer {
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	return &Consumer{repository: repository, config: config}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.repository == nil {
		return errors.New("broadcast execution repository is not configured")
	}

	for {
		if err := c.poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("broadcast execution poll failed", "error", err)
		}

		timer := time.NewTimer(c.config.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func (c *Consumer) poll(ctx context.Context) error {
	if err := c.queueDueBroadcasts(ctx); err != nil {
		return err
	}
	return c.materializeQueuedBroadcasts(ctx)
}

func (c *Consumer) queueDueBroadcasts(ctx context.Context) error {
	for processed := 0; processed < c.config.BatchSize; processed++ {
		broadcast, claimed, err := c.repository.QueueNextDueScheduled(ctx)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		slog.Info(
			"scheduled broadcast queued for execution",
			"broadcast_id", broadcast.ID,
			"team_id", broadcast.TeamID,
			"scheduled_at", broadcast.ScheduledAt,
		)
	}
	return nil
}

func (c *Consumer) materializeQueuedBroadcasts(ctx context.Context) error {
	for processed := 0; processed < c.config.BatchSize; processed++ {
		result, claimed, err := c.repository.MaterializeNextQueuedRecipients(ctx)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		slog.Info(
			"broadcast recipients materialized",
			"broadcast_id", result.BroadcastID,
			"team_id", result.TeamID,
			"audience_count", result.AudienceCount,
			"eligible_count", result.EligibleCount,
			"excluded_count", result.ExcludedCount,
		)
	}
	return nil
}
