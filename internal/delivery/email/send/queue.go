package emaildelivery

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

type eventStore interface {
	Enqueue(context.Context, outbox.Event) (uuid.UUID, error)
	EnqueueTx(context.Context, pgx.Tx, outbox.Event) (uuid.UUID, error)
	DeletePendingTx(context.Context, pgx.Tx, uuid.UUID) error
	UpdatePendingAvailableAtTx(context.Context, pgx.Tx, uuid.UUID, time.Time) error
}

type Queue struct {
	store eventStore
}

func NewQueue(store eventStore) *Queue { return &Queue{store: store} }

func (q *Queue) EnqueueEmailDelivery(ctx context.Context, messageID uuid.UUID, teamID uuid.UUID) error {
	if q == nil || q.store == nil {
		return errors.New("email delivery outbox is not configured")
	}
	event, err := newDeliveryEvent(messageID, teamID)
	if err != nil {
		return err
	}
	_, err = q.store.Enqueue(ctx, event)
	return err
}

func (q *Queue) EnqueueEmailDeliveryTx(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, teamID uuid.UUID) error {
	return q.EnqueueEmailDeliveryAtTx(ctx, tx, messageID, teamID, time.Time{})
}

func (q *Queue) EnqueueEmailDeliveryAtTx(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, teamID uuid.UUID, availableAt time.Time) error {
	if q == nil || q.store == nil {
		return errors.New("email delivery outbox is not configured")
	}
	event, err := newDeliveryEvent(messageID, teamID)
	if err != nil {
		return err
	}
	event.AvailableAt = availableAt
	_, err = q.store.EnqueueTx(ctx, tx, event)
	return err
}

func (q *Queue) RescheduleEmailDeliveryTx(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, _ uuid.UUID, availableAt time.Time) error {
	if q == nil || q.store == nil {
		return errors.New("email delivery outbox is not configured")
	}
	eventID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(deliveryNamespace+messageID.String()))
	return q.store.UpdatePendingAvailableAtTx(ctx, tx, eventID, availableAt)
}

func (q *Queue) CancelEmailDeliveryTx(ctx context.Context, tx pgx.Tx, messageID uuid.UUID, _ uuid.UUID) error {
	if q == nil || q.store == nil {
		return errors.New("email delivery outbox is not configured")
	}
	eventID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(deliveryNamespace+messageID.String()))
	return q.store.DeletePendingTx(ctx, tx, eventID)
}
