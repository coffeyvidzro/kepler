package dispatch

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

type eventStore interface {
	EnqueueTx(context.Context, pgx.Tx, outbox.Event) (uuid.UUID, error)
}

type Queue struct{ store eventStore }

func NewQueue(store eventStore) *Queue { return &Queue{store: store} }

func (queue *Queue) EnqueueVerificationDispatchTx(ctx context.Context, tx pgx.Tx, command Command) error {
	if queue == nil || queue.store == nil {
		return errors.New("verification dispatch outbox is not configured")
	}
	if tx == nil {
		return errors.New("verification dispatch transaction is required")
	}
	event, err := NewEvent(command)
	if err != nil {
		return err
	}
	_, err = queue.store.EnqueueTx(ctx, tx, event)
	return err
}
