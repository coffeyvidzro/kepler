package event

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Result struct {
	EventID       uuid.UUID
	DeliveryCount int64
}

type Sink interface {
	EmitTx(context.Context, pgx.Tx, Envelope) (Result, error)
}

type Emitter struct {
	sink Sink
	now  func() time.Time
}

func NewEmitter(sink Sink) *Emitter {
	return &Emitter{sink: sink, now: time.Now}
}

func (emitter *Emitter) EmitTx(ctx context.Context, tx pgx.Tx, envelope Envelope) (Result, error) {
	if emitter == nil || emitter.sink == nil {
		return Result{}, errors.New("event emitter is not configured")
	}
	if tx == nil {
		return Result{}, errors.New("event transaction is required")
	}
	normalized, err := envelope.Normalize(emitter.now)
	if err != nil {
		return Result{}, err
	}
	result, err := emitter.sink.EmitTx(ctx, tx, normalized)
	if err != nil {
		return Result{}, fmt.Errorf("emit platform event: %w", err)
	}
	if result.EventID == uuid.Nil {
		return Result{}, errors.New("event sink returned an empty event id")
	}
	if result.EventID != normalized.ID {
		return Result{}, errors.New("event sink returned a different event id")
	}
	if result.DeliveryCount < 0 {
		return Result{}, errors.New("event sink returned a negative delivery count")
	}
	return result, nil
}
