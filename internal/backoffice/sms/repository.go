package sms

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
		SELECT
			s.id::text,
			t.name,
			s.to_number,
			s.from_name,
			s.status,
			coalesce(s.provider_id, ''),
			coalesce(s.error_message, ''),
			s.created_at
		FROM sms_messages s
		JOIN teams t ON t.id = s.team_id
		WHERE ($1 = '' OR t.name ILIKE '%' || $1 || '%' OR s.to_number ILIKE '%' || $1 || '%' OR s.from_name ILIKE '%' || $1 || '%')
		  AND ($2 = '' OR s.status = $2)
		ORDER BY s.created_at DESC
		LIMIT 100
	`, filter.Query, filter.Status)
	if err != nil {
		return nil, fmt.Errorf("list sms messages: %w", err)
	}
	defer rows.Close()

	var messages []Row
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.ID, &row.TeamName, &row.ToNumber, &row.FromName, &row.Status, &row.ProviderID, &row.ErrorMessage, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan sms message: %w", err)
		}
		messages = append(messages, row)
	}

	return messages, rows.Err()
}

func (r *Repository) Detail(ctx context.Context, id string) (Detail, error) {
	var detail Detail
	if err := r.db.QueryRow(ctx, `
		SELECT
			s.id::text,
			t.id::text,
			t.name,
			coalesce(s.sender_id::text, ''),
			s.to_number,
			s.from_name,
			s.body,
			s.status,
			coalesce(s.provider_id, ''),
			coalesce(s.provider_message_id, ''),
			s.segments,
			coalesce(s.error_message, ''),
			coalesce(s.metadata::text, '{}'),
			coalesce(to_char(s.submitted_at, 'YYYY-MM-DD HH24:MI'), ''),
			coalesce(to_char(s.delivered_at, 'YYYY-MM-DD HH24:MI'), ''),
			s.created_at,
			s.updated_at
		FROM sms_messages s
		JOIN teams t ON t.id = s.team_id
		WHERE s.id = $1::uuid
	`, id).Scan(
		&detail.ID,
		&detail.TeamID,
		&detail.TeamName,
		&detail.SenderID,
		&detail.ToNumber,
		&detail.FromName,
		&detail.Body,
		&detail.Status,
		&detail.ProviderID,
		&detail.ProviderMessageID,
		&detail.Segments,
		&detail.ErrorMessage,
		&detail.Metadata,
		&detail.SubmittedAt,
		&detail.DeliveredAt,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	); err != nil {
		return Detail{}, fmt.Errorf("get sms detail: %w", err)
	}

	return detail, nil
}
