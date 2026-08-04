package postgres

import (
	"context"

	legacy "github.com/coffeyvidzro/dugble/server/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates and verifies a PostgreSQL connection pool.
func New(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	return legacy.NewPostgres(ctx, databaseURL)
}
