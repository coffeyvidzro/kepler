package segment

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, name string) (Segment, error) {
	var value Segment
	err := r.db.QueryRow(ctx, `INSERT INTO segments (team_id, name)
		VALUES ($1, $2)
		RETURNING id, team_id, name, created_at`, teamID, name).Scan(
		&value.ID, &value.TeamID, &value.Name, &value.CreatedAt,
	)
	if err != nil {
		return Segment{}, fmt.Errorf("create segment: %w", err)
	}
	return value, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Segment, error) {
	rows, err := r.db.Query(ctx, `SELECT id, team_id, name, created_at
		FROM segments
		WHERE team_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, teamID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}
	defer rows.Close()

	values := make([]Segment, 0)
	for rows.Next() {
		var value Segment
		if err := rows.Scan(&value.ID, &value.TeamID, &value.Name, &value.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan segment: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate segments: %w", err)
	}
	return values, nil
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Segment, error) {
	var value Segment
	err := r.db.QueryRow(ctx, `SELECT id, team_id, name, created_at
		FROM segments
		WHERE id = $1 AND team_id = $2`, id, teamID).Scan(
		&value.ID, &value.TeamID, &value.Name, &value.CreatedAt,
	)
	if err != nil {
		return Segment{}, err
	}
	return value, nil
}
