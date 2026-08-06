package teams

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
		SELECT id::text, name, status, created_at
		FROM teams
		WHERE $1 = '' OR name ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC
		LIMIT 100
	`, filter.Query)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()

	var teams []Row
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.ID, &row.Name, &row.Status, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		teams = append(teams, row)
	}

	return teams, rows.Err()
}

func (r *Repository) Detail(ctx context.Context, id string) (Detail, error) {
	var detail Detail
	if err := r.db.QueryRow(ctx, `
		SELECT id::text, name, status, created_at
		FROM teams
		WHERE id = $1::uuid
	`, id).Scan(&detail.Team.ID, &detail.Team.Name, &detail.Team.Status, &detail.Team.CreatedAt); err != nil {
		return Detail{}, fmt.Errorf("get team detail: %w", err)
	}

	if err := r.loadMembers(ctx, id, &detail); err != nil {
		return Detail{}, err
	}
	if err := r.loadSMS(ctx, id, &detail); err != nil {
		return Detail{}, err
	}

	return detail, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE teams
		SET status = $2,
			updated_at = now()
		WHERE id = $1::uuid
	`, id, status)
	if err != nil {
		return fmt.Errorf("update team status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update team status: %w", pgx.ErrNoRows)
	}

	return nil
}

func (r *Repository) loadMembers(ctx context.Context, id string, detail *Detail) error {
	members, err := r.db.Query(ctx, `
		SELECT u.id::text, u.email, u.name, tm.role, tm.status, tm.created_at
		FROM team_members tm
		JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = $1::uuid
		ORDER BY tm.created_at DESC
	`, id)
	if err != nil {
		return fmt.Errorf("list team members: %w", err)
	}
	defer members.Close()

	for members.Next() {
		var row MemberRow
		if err := members.Scan(&row.UserID, &row.Email, &row.Name, &row.Role, &row.Status, &row.CreatedAt); err != nil {
			return fmt.Errorf("scan team member: %w", err)
		}
		detail.Members = append(detail.Members, row)
	}

	return members.Err()
}

func (r *Repository) loadSMS(ctx context.Context, id string, detail *Detail) error {
	smsRows, err := r.db.Query(ctx, `
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
		WHERE s.team_id = $1::uuid
		ORDER BY s.created_at DESC
		LIMIT 25
	`, id)
	if err != nil {
		return fmt.Errorf("list team sms messages: %w", err)
	}
	defer smsRows.Close()

	for smsRows.Next() {
		var row SMSRow
		if err := smsRows.Scan(&row.ID, &row.TeamName, &row.ToNumber, &row.FromName, &row.Status, &row.ProviderID, &row.ErrorMessage, &row.CreatedAt); err != nil {
			return fmt.Errorf("scan team sms message: %w", err)
		}
		detail.SMS = append(detail.SMS, row)
	}

	return smsRows.Err()
}
