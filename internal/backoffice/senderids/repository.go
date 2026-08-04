package senderids

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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
		SELECT s.id::text, t.name, s.name, s.country_code, s.status, s.created_at
		FROM sender_ids s
		JOIN teams t ON t.id = s.team_id
		WHERE ($1 = '' OR t.name ILIKE '%' || $1 || '%' OR s.name ILIKE '%' || $1 || '%' OR s.country_code ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR s.status = $2)
		ORDER BY s.created_at DESC
		LIMIT 100
	`, filter.Query, filter.Status)
	if err != nil {
		return nil, fmt.Errorf("list sender ids: %w", err)
	}
	defer rows.Close()

	var senderIDs []Row
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.ID, &row.TeamName, &row.Name, &row.CountryCode, &row.Status, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan sender id: %w", err)
		}
		senderIDs = append(senderIDs, row)
	}

	return senderIDs, rows.Err()
}

func (r *Repository) Detail(ctx context.Context, id string) (Detail, error) {
	var detail Detail
	if err := r.db.QueryRow(ctx, `
		SELECT
			s.id::text,
			s.team_id::text,
			t.name,
			s.name,
			s.country_code,
			s.purpose,
			s.status,
			coalesce(s.provider, ''),
			coalesce(s.rejection_reason, ''),
			coalesce(to_char(s.approved_at, 'YYYY-MM-DD HH24:MI'), ''),
			coalesce(to_char(s.rejected_at, 'YYYY-MM-DD HH24:MI'), ''),
			coalesce(to_char(s.suspended_at, 'YYYY-MM-DD HH24:MI'), ''),
			coalesce(s.created_by::text, ''),
			s.created_at,
			s.updated_at
		FROM sender_ids s
		JOIN teams t ON t.id = s.team_id
		WHERE s.id = $1::uuid
	`, id).Scan(
		&detail.ID,
		&detail.TeamID,
		&detail.TeamName,
		&detail.Name,
		&detail.CountryCode,
		&detail.Purpose,
		&detail.Status,
		&detail.Provider,
		&detail.RejectionReason,
		&detail.ApprovedAt,
		&detail.RejectedAt,
		&detail.SuspendedAt,
		&detail.CreatedBy,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	); err != nil {
		return Detail{}, fmt.Errorf("get sender id detail: %w", err)
	}

	return detail, nil
}

func (r *Repository) Approve(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE sender_ids
		SET status = 'approved',
			approved_at = now(),
			rejected_at = NULL,
			rejection_reason = NULL,
			updated_at = now()
		WHERE id = $1::uuid
	`, id)
	if err != nil {
		return fmt.Errorf("approve sender id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("approve sender id: %w", pgx.ErrNoRows)
	}

	return nil
}

func (r *Repository) Reject(ctx context.Context, id string, reason string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE sender_ids
		SET status = 'rejected',
			rejected_at = now(),
			approved_at = NULL,
			rejection_reason = $2,
			updated_at = now()
		WHERE id = $1::uuid
	`, id, reason)
	if err != nil {
		return fmt.Errorf("reject sender id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("reject sender id: %w", pgx.ErrNoRows)
	}

	return nil
}
