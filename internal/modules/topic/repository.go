package topic

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, teamID uuid.UUID, req CreateRequest) (Topic, error) {
	var value Topic
	err := r.db.QueryRow(ctx, `
		INSERT INTO topics (team_id, name, description, default_subscription, visibility)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, team_id, name, description, default_subscription, visibility, created_at, updated_at
	`, teamID, req.Name, req.Description, req.DefaultSubscription, req.Visibility).Scan(
		&value.ID, &value.TeamID, &value.Name, &value.Description,
		&value.DefaultSubscription, &value.Visibility, &value.CreatedAt, &value.UpdatedAt,
	)
	if err != nil {
		return Topic{}, fmt.Errorf("create topic: %w", err)
	}
	return value, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Topic, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, team_id, name, description, default_subscription, visibility, created_at, updated_at
		FROM topics WHERE team_id = $1
		ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3
	`, teamID, limit, offset)
	if err != nil { return nil, fmt.Errorf("list topics: %w", err) }
	defer rows.Close()
	values := make([]Topic, 0)
	for rows.Next() {
		var value Topic
		if err := rows.Scan(&value.ID, &value.TeamID, &value.Name, &value.Description, &value.DefaultSubscription, &value.Visibility, &value.CreatedAt, &value.UpdatedAt); err != nil { return nil, fmt.Errorf("scan topic: %w", err) }
		values = append(values, value)
	}
	return values, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (Topic, error) {
	var value Topic
	err := r.db.QueryRow(ctx, `
		SELECT id, team_id, name, description, default_subscription, visibility, created_at, updated_at
		FROM topics WHERE id = $1 AND team_id = $2
	`, id, teamID).Scan(&value.ID, &value.TeamID, &value.Name, &value.Description, &value.DefaultSubscription, &value.Visibility, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (r *Repository) Update(ctx context.Context, id, teamID uuid.UUID, name string, description *string) (Topic, error) {
	var value Topic
	err := r.db.QueryRow(ctx, `
		UPDATE topics SET name = $3, description = $4, updated_at = now()
		WHERE id = $1 AND team_id = $2
		RETURNING id, team_id, name, description, default_subscription, visibility, created_at, updated_at
	`, id, teamID, name, description).Scan(&value.ID, &value.TeamID, &value.Name, &value.Description, &value.DefaultSubscription, &value.Visibility, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func (r *Repository) Delete(ctx context.Context, id, teamID uuid.UUID) (Topic, error) {
	var value Topic
	err := r.db.QueryRow(ctx, `
		DELETE FROM topics WHERE id = $1 AND team_id = $2
		RETURNING id, team_id, name, description, default_subscription, visibility, created_at, updated_at
	`, id, teamID).Scan(&value.ID, &value.TeamID, &value.Name, &value.Description, &value.DefaultSubscription, &value.Visibility, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}
