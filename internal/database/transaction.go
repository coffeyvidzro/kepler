package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrNilTransactionOperation = errors.New("transaction operation is required")

// TransactionBeginner is implemented by pgxpool.Pool and pgx.Conn.
type TransactionBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// InTransaction runs operation in a transaction using the default pgx options.
func InTransaction(ctx context.Context, beginner TransactionBeginner, operation func(pgx.Tx) error) error {
	return InTransactionWithOptions(ctx, beginner, pgx.TxOptions{}, operation)
}

// InTransactionWithOptions runs operation in a transaction and commits only when
// operation succeeds. The deferred rollback also runs during panic unwinding; pgx
// safely returns pgx.ErrTxClosed when the transaction was already committed.
func InTransactionWithOptions(
	ctx context.Context,
	beginner TransactionBeginner,
	options pgx.TxOptions,
	operation func(pgx.Tx) error,
) error {
	if operation == nil {
		return ErrNilTransactionOperation
	}
	if beginner == nil {
		return errors.New("transaction beginner is required")
	}

	tx, err := beginner.BeginTx(ctx, options)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := operation(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// InTransactionResult is the result-returning form of InTransaction.
func InTransactionResult[T any](
	ctx context.Context,
	beginner TransactionBeginner,
	operation func(pgx.Tx) (T, error),
) (T, error) {
	var result T
	if operation == nil {
		return result, ErrNilTransactionOperation
	}
	err := InTransaction(ctx, beginner, func(tx pgx.Tx) error {
		var err error
		result, err = operation(tx)
		return err
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}
