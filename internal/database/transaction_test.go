package database

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestInTransactionCommitsSuccessfulOperation(t *testing.T) {
	tx := &testTransaction{}
	beginner := &testTransactionBeginner{tx: tx}
	called := false

	err := InTransaction(t.Context(), beginner, func(got pgx.Tx) error {
		called = true
		if got != tx {
			t.Fatal("operation received a different transaction")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("InTransaction returned error: %v", err)
	}
	if !called || tx.commitCalls != 1 || tx.rollbackCalls != 1 {
		t.Fatalf("called=%t commits=%d rollbacks=%d, want true, 1, 1", called, tx.commitCalls, tx.rollbackCalls)
	}
}

func TestInTransactionRollsBackFailedOperation(t *testing.T) {
	operationErr := errors.New("operation failed")
	tx := &testTransaction{}
	err := InTransaction(t.Context(), &testTransactionBeginner{tx: tx}, func(pgx.Tx) error {
		return operationErr
	})
	if !errors.Is(err, operationErr) {
		t.Fatalf("InTransaction error = %v, want %v", err, operationErr)
	}
	if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("commits=%d rollbacks=%d, want 0, 1", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestInTransactionReturnsBeginAndCommitErrors(t *testing.T) {
	beginErr := errors.New("begin failed")
	called := false
	err := InTransaction(t.Context(), &testTransactionBeginner{err: beginErr}, func(pgx.Tx) error {
		called = true
		return nil
	})
	if !errors.Is(err, beginErr) || called {
		t.Fatalf("begin failure returned error=%v called=%t", err, called)
	}

	commitErr := errors.New("commit failed")
	tx := &testTransaction{commitErr: commitErr}
	err = InTransaction(t.Context(), &testTransactionBeginner{tx: tx}, func(pgx.Tx) error { return nil })
	if !errors.Is(err, commitErr) {
		t.Fatalf("commit failure returned error=%v, want %v", err, commitErr)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("rollback calls = %d, want 1", tx.rollbackCalls)
	}
}

func TestInTransactionRollsBackWhenOperationPanics(t *testing.T) {
	tx := &testTransaction{}
	defer func() {
		if recover() == nil {
			t.Fatal("expected operation panic")
		}
		if tx.rollbackCalls != 1 {
			t.Fatalf("rollback calls = %d, want 1", tx.rollbackCalls)
		}
	}()
	_ = InTransaction(t.Context(), &testTransactionBeginner{tx: tx}, func(pgx.Tx) error {
		panic("boom")
	})
}

func TestInTransactionPassesOptionsAndReturnsResult(t *testing.T) {
	options := pgx.TxOptions{IsoLevel: pgx.Serializable, AccessMode: pgx.ReadOnly}
	tx := &testTransaction{}
	beginner := &testTransactionBeginner{tx: tx}
	if err := InTransactionWithOptions(t.Context(), beginner, options, func(pgx.Tx) error { return nil }); err != nil {
		t.Fatalf("InTransactionWithOptions returned error: %v", err)
	}
	if beginner.options != options {
		t.Fatalf("options = %+v, want %+v", beginner.options, options)
	}

	result, err := InTransactionResult(t.Context(), &testTransactionBeginner{tx: &testTransaction{}}, func(pgx.Tx) (string, error) {
		return "committed", nil
	})
	if err != nil || result != "committed" {
		t.Fatalf("result=%q error=%v, want committed, nil", result, err)
	}
}

func TestInTransactionRejectsMissingDependencies(t *testing.T) {
	if err := InTransaction(t.Context(), nil, func(pgx.Tx) error { return nil }); err == nil {
		t.Fatal("expected nil beginner error")
	}
	if err := InTransaction(t.Context(), &testTransactionBeginner{}, nil); !errors.Is(err, ErrNilTransactionOperation) {
		t.Fatalf("nil operation error = %v, want %v", err, ErrNilTransactionOperation)
	}
}

type testTransactionBeginner struct {
	tx      pgx.Tx
	err     error
	options pgx.TxOptions
}

func (b *testTransactionBeginner) BeginTx(_ context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	b.options = options
	return b.tx, b.err
}

type testTransaction struct {
	pgx.Tx
	commitErr     error
	commitCalls   int
	rollbackCalls int
}

func (t *testTransaction) Commit(context.Context) error {
	t.commitCalls++
	return t.commitErr
}

func (t *testTransaction) Rollback(context.Context) error {
	t.rollbackCalls++
	return nil
}
