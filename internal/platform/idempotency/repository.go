package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

const (
	tryAcquireLeaseSQL  = `SELECT pg_try_advisory_lock(hashtextextended($1::text, 0))`
	releaseLeaseSQL     = `SELECT pg_advisory_unlock(hashtextextended($1::text, 0))`
	leaseReleaseTimeout = 5 * time.Second
)

var (
	ErrAlreadyExists = errors.New("idempotency key already exists")
	ErrNotFound      = errors.New("idempotency key not found")
)

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (r *Repository) TryAcquireLease(
	ctx context.Context,
	scope string,
	key string,
) (Lease, bool, error) {
	conn, err := r.db.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire idempotency lease connection: %w", err)
	}

	identity := leaseIdentity(scope, key)
	var acquired bool
	if err := conn.QueryRow(ctx, tryAcquireLeaseSQL, identity).Scan(&acquired); err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("acquire idempotency lease: %w", err)
	}
	if !acquired {
		conn.Release()
		return nil, false, nil
	}

	return &postgresLease{conn: conn, identity: identity}, true, nil
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
		if errors.Is(err, pgx.ErrNoRows) {
			return Record{}, ErrNotFound
		}
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

type postgresLease struct {
	conn     *pgxpool.Conn
	identity string
}

func (l *postgresLease) Release(_ context.Context) error {
	if l == nil || l.conn == nil {
		return nil
	}

	conn := l.conn
	l.conn = nil

	ctx, cancel := context.WithTimeout(context.Background(), leaseReleaseTimeout)
	defer cancel()

	var unlocked bool
	err := conn.QueryRow(ctx, releaseLeaseSQL, l.identity).Scan(&unlocked)
	if err == nil && unlocked {
		conn.Release()
		return nil
	}

	closeCtx, closeCancel := context.WithTimeout(context.Background(), leaseReleaseTimeout)
	closeErr := conn.Conn().Close(closeCtx)
	closeCancel()
	conn.Release()

	if err != nil {
		return fmt.Errorf("release idempotency lease: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("discard idempotency lease connection: %w", closeErr)
	}
	return errors.New("release idempotency lease: advisory lock was not held")
}

func leaseIdentity(scope string, key string) string {
	return fmt.Sprintf("%d:%s%s", len(scope), scope, key)
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
