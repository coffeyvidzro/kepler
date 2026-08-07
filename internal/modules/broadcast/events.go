package broadcast

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type eventEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformevent.Envelope) (platformevent.Result, error)
}

type broadcastTransition struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

type broadcastSummary struct {
	AudienceCount   int64 `json:"audience_count"`
	EligibleCount   int64 `json:"eligible_count"`
	SuppressedCount int64 `json:"suppressed_count"`
	QueuedCount     int64 `json:"queued_count"`
	FailedCount     int64 `json:"failed_count"`
}

func emitBroadcastEvent(ctx context.Context, tx pgx.Tx, emitter eventEmitter, eventType platformevent.Type, value Broadcast, from, reason string, failure map[string]any) error {
	if emitter == nil {
		emitter = platformevent.DefaultEmitter()
	}
	if emitter == nil {
		return nil
	}
	teamID, err := uuid.Parse(value.TeamID)
	if err != nil {
		return fmt.Errorf("parse broadcast team id: %w", err)
	}
	objectID, err := uuid.Parse(value.ID)
	if err != nil {
		return fmt.Errorf("parse broadcast id: %w", err)
	}
	payload := map[string]any{
		"broadcast": value,
		"transition": broadcastTransition{From: from, To: value.Status, Reason: reason},
		"summary": broadcastSummary{
			AudienceCount: value.AudienceCount, EligibleCount: value.EligibleCount,
			SuppressedCount: value.SuppressedCount, QueuedCount: value.QueuedCount,
			FailedCount: value.FailedCount,
		},
	}
	if failure != nil {
		payload["failure"] = failure
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode broadcast event: %w", err)
	}
	_, err = emitter.EmitTx(ctx, tx, platformevent.Envelope{
		Type:       eventType,
		TeamID:     teamID,
		ObjectType: "broadcast",
		ObjectID:   &objectID,
		Data:       data,
	})
	return err
}
