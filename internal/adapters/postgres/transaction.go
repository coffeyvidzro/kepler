package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var (
	ErrTransactionContextRequired  = errors.New("transaction context is required")
	ErrTransactionBeginnerRequired = errors.New("transaction beginner is required")
	ErrNilTransactionOperation     = errors.New("transaction operation is required")
)

type TransactionBeginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

func InTransaction(
	ctx context.Context,
	beginner TransactionBeginner,
	operation func(pgx.Tx) error,
) error {
	return InTransactionWithOptions(ctx, beginner, pgx.TxOptions{}, operation)
}

func InTransactionWithOptions(
	ctx context.Context,
	beginner TransactionBeginner,
	options pgx.TxOptions,
	operation func(pgx.Tx) error,
) error {
	if ctx == nil {
		return ErrTransactionContextRequired
	}
	if beginner == nil {
		return ErrTransactionBeginnerRequired
	}
	if operation == nil {
		return ErrNilTransactionOperation
	}
	tx, err := beginner.BeginTx(ctx, options)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		rollbackContext := context.Background()
		if ctx != nil {
			rollbackContext = context.WithoutCancel(ctx)
		}
		_ = tx.Rollback(rollbackContext)
	}()

	if err := operation(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

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
