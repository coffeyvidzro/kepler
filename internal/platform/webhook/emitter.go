package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Store interface {
	CreateEventTx(context.Context, pgx.Tx, Event) (uuid.UUID, error)
	CreateDeliveriesForEventTx(context.Context, pgx.Tx, uuid.UUID, time.Time) (int64, error)
	CreateDeliveryTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, time.Time) (uuid.UUID, error)
}

type Emitter struct {
	store Store
	now   func() time.Time
}

func NewEmitter(store Store) *Emitter {
	return &Emitter{store: store, now: time.Now}
}

func (e *Emitter) EmitTx(ctx context.Context, tx pgx.Tx, event Event) (uuid.UUID, int64, error) {
	event, err := e.normalize(event)
	if err != nil {
		return uuid.Nil, 0, err
	}
	if tx == nil {
		return uuid.Nil, 0, errors.New("webhook transaction is required")
	}

	eventID, err := e.store.CreateEventTx(ctx, tx, event)
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf("create webhook event: %w", err)
	}
	count, err := e.store.CreateDeliveriesForEventTx(ctx, tx, eventID, e.now().UTC())
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf("create webhook deliveries: %w", err)
	}
	return eventID, count, nil
}

func (e *Emitter) EmitToEndpointTx(
	ctx context.Context,
	tx pgx.Tx,
	event Event,
	endpointID uuid.UUID,
) (uuid.UUID, uuid.UUID, error) {
	event, err := e.normalize(event)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if tx == nil {
		return uuid.Nil, uuid.Nil, errors.New("webhook transaction is required")
	}
	if endpointID == uuid.Nil {
		return uuid.Nil, uuid.Nil, errors.New("webhook endpoint id is required")
	}

	eventID, err := e.store.CreateEventTx(ctx, tx, event)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("create webhook event: %w", err)
	}
	deliveryID, err := e.store.CreateDeliveryTx(ctx, tx, eventID, endpointID, e.now().UTC())
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("create webhook delivery: %w", err)
	}
	return eventID, deliveryID, nil
}

func (e *Emitter) normalize(event Event) (Event, error) {
	if e == nil || e.store == nil {
		return Event{}, errors.New("webhook emitter is not configured")
	}
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.TeamID == uuid.Nil {
		return Event{}, errors.New("webhook team id is required")
	}
	event.Type = strings.TrimSpace(event.Type)
	if event.Type == "" {
		return Event{}, errors.New("webhook event type is required")
	}
	event.ObjectType = strings.TrimSpace(event.ObjectType)
	if event.ObjectType == "" {
		return Event{}, errors.New("webhook object type is required")
	}
	if !json.Valid(event.Payload) || !bytes.HasPrefix(bytes.TrimSpace(event.Payload), []byte("{")) {
		return Event{}, errors.New("webhook payload must be a JSON object")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = e.now().UTC()
	} else {
		event.OccurredAt = event.OccurredAt.UTC()
	}
	return event, nil
}
