package users

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context, filter Filter) ([]Row, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, email, name, email_verified, created_at
		FROM users
		WHERE $1 = '' OR email ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC
		LIMIT 100
	`, filter.Query)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []Row
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.ID, &row.Email, &row.Name, &row.EmailVerified, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, row)
	}

	return users, rows.Err()
}

func (r *Repository) Detail(ctx context.Context, id string) (Detail, error) {
	var detail Detail
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, email, name, email_verified, created_at
		FROM users
		WHERE id = $1::uuid
	`, id).Scan(&detail.User.ID, &detail.User.Email, &detail.User.Name, &detail.User.EmailVerified, &detail.User.CreatedAt); err != nil {
		return Detail{}, fmt.Errorf("get user detail: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT t.id::text, t.name, tm.role, tm.status
		FROM team_members tm
		JOIN teams t ON t.id = tm.team_id
		WHERE tm.user_id = $1::uuid
		ORDER BY t.created_at DESC
	`, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list user teams: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var row TeamMembershipRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Role, &row.Status); err != nil {
			return Detail{}, fmt.Errorf("scan user team: %w", err)
		}
		detail.Teams = append(detail.Teams, row)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, err
	}

	return detail, nil
}
