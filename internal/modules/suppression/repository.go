package suppression

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAlreadyExists = errors.New("suppression already exists")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, email string) (Suppression, error) {
	var value Suppression
	err := r.db.QueryRow(ctx, `
		INSERT INTO suppressions (team_id, email, origin)
		VALUES ($1, $2, 'manual')
		RETURNING id, team_id, email, origin, source_id, created_at
	`, teamID, email).Scan(&value.ID, &value.TeamID, &value.Email, &value.Origin, &value.SourceID, &value.CreatedAt)
	if isUniqueViolation(err) {
		return Suppression{}, ErrAlreadyExists
	}
	if err != nil {
		return Suppression{}, fmt.Errorf("create suppression: %w", err)
	}
	return value, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Suppression, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, email, origin, source_id, created_at
		FROM suppressions WHERE team_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list suppressions: %w", err)
	}
	defer rows.Close()
	values := make([]Suppression, 0)
	for rows.Next() {
		var value Suppression
		if err := rows.Scan(&value.ID, &value.TeamID, &value.Email, &value.Origin, &value.SourceID, &value.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan suppression: %w", err)
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *Repository) GetByID(ctx context.Context, id, teamID uuid.UUID) (Suppression, error) {
	var value Suppression
	err := r.db.QueryRow(ctx, `SELECT id, team_id, email, origin, source_id, created_at FROM suppressions WHERE id = $1 AND team_id = $2`, id, teamID).Scan(&value.ID, &value.TeamID, &value.Email, &value.Origin, &value.SourceID, &value.CreatedAt)
	return value, err
}

func (r *Repository) GetByEmail(ctx context.Context, email string, teamID uuid.UUID) (Suppression, error) {
	var value Suppression
	err := r.db.QueryRow(ctx, `SELECT id, team_id, email, origin, source_id, created_at FROM suppressions WHERE team_id = $1 AND lower(email) = lower($2)`, teamID, email).Scan(&value.ID, &value.TeamID, &value.Email, &value.Origin, &value.SourceID, &value.CreatedAt)
	return value, err
}

func (r *Repository) DeleteByID(ctx context.Context, id, teamID uuid.UUID) (Suppression, error) {
	var value Suppression
	err := r.db.QueryRow(ctx, `DELETE FROM suppressions WHERE id = $1 AND team_id = $2 RETURNING id, team_id, email, origin, source_id, created_at`, id, teamID).Scan(&value.ID, &value.TeamID, &value.Email, &value.Origin, &value.SourceID, &value.CreatedAt)
	return value, err
}

func (r *Repository) DeleteByEmail(ctx context.Context, email string, teamID uuid.UUID) (Suppression, error) {
	var value Suppression
	err := r.db.QueryRow(ctx, `DELETE FROM suppressions WHERE team_id = $1 AND lower(email) = lower($2) RETURNING id, team_id, email, origin, source_id, created_at`, teamID, email).Scan(&value.ID, &value.TeamID, &value.Email, &value.Origin, &value.SourceID, &value.CreatedAt)
	return value, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && strings.EqualFold(pgErr.Code, "23505")
}
