package webhook

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

// EventSink adapts webhook persistence to the canonical platform event sink.
type EventSink struct {
	emitter *Emitter
}

func NewEventSink(emitter *Emitter) *EventSink {
	return &EventSink{emitter: emitter}
}

func (sink *EventSink) EmitTx(
	ctx context.Context,
	tx pgx.Tx,
	envelope platformevent.Envelope,
) (platformevent.Result, error) {
	if sink == nil || sink.emitter == nil {
		return platformevent.Result{}, errors.New("webhook event sink is not configured")
	}
	return sink.emitter.emitEnvelopeTx(ctx, tx, envelope)
}
