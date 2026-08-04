package idempotency

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

var ErrAlreadyExists = errors.New("idempotency key already exists")

type Repository struct {
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{queries: dbsqlc.New(db)}
}

func (r *Repository) CreateProcessing(ctx context.Context, record Record) (Record, error) {
	created, err := r.queries.CreateIdempotencyKey(ctx, dbsqlc.CreateIdempotencyKeyParams{
		Scope:          record.Scope,
		IdempotencyKey: record.Key,
		Method:         record.Method,
		Path:           record.Path,
		RequestHash:    record.RequestHash,
		LockedUntil:    pgconv.NullableTimestamptz(&record.LockedUntil),
		ExpiresAt:      pgconv.NullableTimestamptz(&record.ExpiresAt),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrAlreadyExists
		}

		return Record{}, fmt.Errorf("create idempotency key: %w", err)
	}

	return recordFromSQLC(created), nil
}

func (r *Repository) Get(ctx context.Context, scope string, key string) (Record, error) {
	record, err := r.queries.GetIdempotencyKey(
		ctx,
		dbsqlc.GetIdempotencyKeyParams{Scope: scope, IdempotencyKey: key},
	)
	if err != nil {
		return Record{}, fmt.Errorf("get idempotency key: %w", err)
	}

	return recordFromSQLC(record), nil
}

func (r *Repository) Complete(ctx context.Context, scope string, key string, responseStatus int, responseBody []byte, contentType string, responseHeaders []byte) error {
	status := int32(responseStatus)
	if err := r.queries.CompleteIdempotencyKey(ctx, dbsqlc.CompleteIdempotencyKeyParams{
		Scope:               scope,
		IdempotencyKey:      key,
		ResponseStatus:      &status,
		ResponseBody:        responseBody,
		ResponseContentType: optionalString(contentType),
		ResponseHeaders:     responseHeaders,
	}); err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}

	return nil
}

func (r *Repository) Delete(ctx context.Context, scope string, key string) error {
	if err := r.queries.DeleteIdempotencyKey(ctx, dbsqlc.DeleteIdempotencyKeyParams{Scope: scope, IdempotencyKey: key}); err != nil {
		return fmt.Errorf("delete idempotency key: %w", err)
	}

	return nil
}

func recordFromSQLC(row dbsqlc.IdempotencyKey) Record {
	return Record{
		Scope:               row.Scope,
		Key:                 row.IdempotencyKey,
		Method:              row.Method,
		Path:                row.Path,
		RequestHash:         row.RequestHash,
		Status:              row.Status,
		ResponseStatus:      row.ResponseStatus,
		ResponseBody:        row.ResponseBody,
		ResponseContentType: row.ResponseContentType,
		ResponseHeaders:     row.ResponseHeaders,
		LockedUntil:         row.LockedUntil.Time,
		CompletedAt:         pgconv.TimestamptzToTimePtr(row.CompletedAt),
		ExpiresAt:           row.ExpiresAt.Time,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
