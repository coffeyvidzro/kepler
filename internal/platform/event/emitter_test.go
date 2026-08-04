package event

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

type testSink struct{}

func (testSink) EmitTx(context.Context, pgx.Tx, Envelope) (Result, error) {
	return Result{}, nil
}

func TestEmitterRequiresConfiguration(t *testing.T) {
	var emitter *Emitter
	if _, err := emitter.EmitTx(context.Background(), nil, Envelope{}); err == nil {
		t.Fatal("EmitTx() accepted a nil emitter")
	}
	if _, err := NewEmitter(nil).EmitTx(context.Background(), nil, Envelope{}); err == nil {
		t.Fatal("EmitTx() accepted a nil sink")
	}
}

func TestEmitterRequiresTransaction(t *testing.T) {
	if _, err := NewEmitter(testSink{}).EmitTx(context.Background(), nil, Envelope{}); err == nil {
		t.Fatal("EmitTx() accepted a nil transaction")
	}
}
