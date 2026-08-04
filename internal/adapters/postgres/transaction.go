package postgres

import (
	"context"

	legacy "github.com/coffeyvidzro/dugble/server/internal/database"
	"github.com/jackc/pgx/v5"
)

var ErrNilTransactionOperation = legacy.ErrNilTransactionOperation

type TransactionBeginner = legacy.TransactionBeginner

func InTransaction(ctx context.Context, beginner TransactionBeginner, operation func(pgx.Tx) error) error {
	return legacy.InTransaction(ctx, beginner, operation)
}

func InTransactionWithOptions(ctx context.Context, beginner TransactionBeginner, options pgx.TxOptions, operation func(pgx.Tx) error) error {
	return legacy.InTransactionWithOptions(ctx, beginner, options, operation)
}

func InTransactionResult[T any](ctx context.Context, beginner TransactionBeginner, operation func(pgx.Tx) (T, error)) (T, error) {
	return legacy.InTransactionResult(ctx, beginner, operation)
}
