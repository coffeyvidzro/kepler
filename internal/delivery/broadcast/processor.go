package broadcastexecution

import (
	"context"
	"log/slog"

	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
)

type repository interface {
	QueueNextDueScheduled(context.Context) (broadcastmodule.Broadcast, bool, error)
	MaterializeNextQueuedRecipients(context.Context) (broadcastmodule.MaterializationResult, bool, error)
}

type Processor struct {
	repository repository
}

func NewProcessor(repository repository) *Processor {
	return &Processor{repository: repository}
}

func (p *Processor) ProcessBatch(ctx context.Context, batchSize int) error {
	if p == nil || p.repository == nil {
		return ErrProcessorNotConfigured
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if err := p.queueDueBroadcasts(ctx, batchSize); err != nil {
		return err
	}
	return p.materializeQueuedBroadcasts(ctx, batchSize)
}

func (p *Processor) queueDueBroadcasts(ctx context.Context, batchSize int) error {
	for processed := 0; processed < batchSize; processed++ {
		broadcast, claimed, err := p.repository.QueueNextDueScheduled(ctx)
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

func (p *Processor) materializeQueuedBroadcasts(ctx context.Context, batchSize int) error {
	for processed := 0; processed < batchSize; processed++ {
		result, claimed, err := p.repository.MaterializeNextQueuedRecipients(ctx)
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
