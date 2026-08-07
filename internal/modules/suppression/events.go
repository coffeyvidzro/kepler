package suppression

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

func emitSuppressionEvent(ctx context.Context, tx pgx.Tx, emitter eventEmitter, eventType platformevent.Type, value Suppression) error {
	if emitter == nil {
		emitter = platformevent.DefaultEmitter()
	}
	if emitter == nil {
		return nil
	}
	teamID, err := uuid.Parse(value.TeamID)
	if err != nil {
		return fmt.Errorf("parse suppression team id: %w", err)
	}
	objectID, err := uuid.Parse(value.ID)
	if err != nil {
		return fmt.Errorf("parse suppression id: %w", err)
	}
	data, err := json.Marshal(map[string]any{"suppression": value})
	if err != nil {
		return fmt.Errorf("encode suppression event: %w", err)
	}
	_, err = emitter.EmitTx(ctx, tx, platformevent.Envelope{
		Type:       eventType,
		TeamID:     teamID,
		ObjectType: "suppression",
		ObjectID:   &objectID,
		Data:       data,
	})
	return err
}
