package webhook

import (
	"context"
	"testing"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

func TestEventSinkRequiresEmitter(t *testing.T) {
	var sink *EventSink
	if _, err := sink.EmitTx(context.Background(), nil, platformevent.Envelope{}); err == nil {
		t.Fatal("EmitTx() accepted a nil event sink")
	}
	if _, err := NewEventSink(nil).EmitTx(context.Background(), nil, platformevent.Envelope{}); err == nil {
		t.Fatal("EmitTx() accepted a nil webhook emitter")
	}
}
