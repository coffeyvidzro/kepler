package senderid

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSenderIDAlreadyExists = errors.New("sender id already exists")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

const senderIDProjection = `
	sender_id.id,
	sender_id.team_id,
	sender_id.name,
	sender_id.country_code::text,
	COALESCE(sender_id.purpose, ''),
	sender_id.status,
	sender_id.provider,
	sender_id.rejection_reason,
	sender_id.approved_at,
	sender_id.rejected_at,
	sender_id.suspended_at,
	sender_id.created_by,
	sender_id.created_at,
	sender_id.updated_at`

func (r *Repository) Create(
	ctx context.Context,
	teamID uuid.UUID,
	name string,
	countryCode string,
	purpose string,
	provider *string,
	createdBy uuid.UUID,
) (SenderID, error) {
	if r == nil || r.db == nil {
		return SenderID{}, errors.New("sender id repository is not configured")
	}

	normalizedName := strings.ToLower(strings.TrimSpace(name))
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	var normalizedProvider *string
	if provider != nil {
		value := strings.ToLower(strings.TrimSpace(*provider))
		if value != "" {
			normalizedProvider = &value
		}
	}

	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
		INSERT INTO sender_ids (
			team_id, name, normalized_name, country_code, purpose, provider, created_by
		)
		SELECT team.id, $2, $3, $4, NULLIF(trim($5), ''), $6, $7
		FROM teams AS team
		WHERE team.id = $1
		  AND team.status = 'active'
		RETURNING id
	`, teamID, strings.TrimSpace(name), normalizedName, countryCode, purpose,
		normalizedProvider, createdBy).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return SenderID{}, ErrSenderIDAlreadyExists
		}
		return SenderID{}, fmt.Errorf("create sender id: %w", err)
	}

	return getSenderID(ctx, r.db, id, teamID, false)
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]SenderID, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sender id repository is not configured")
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+senderIDProjection+`
		FROM sender_ids AS sender_id
		JOIN teams AS team ON team.id = sender_id.team_id
		WHERE sender_id.team_id = $1
		  AND team.status = 'active'
		ORDER BY sender_id.created_at DESC
	`, teamID)
	if err != nil {
		return nil, fmt.Errorf("list sender ids: %w", err)
	}
	defer rows.Close()

	senders := make([]SenderID, 0)
	for rows.Next() {
		sender, err := scanSenderID(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sender id: %w", err)
		}
		senders = append(senders, sender)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sender ids: %w", err)
	}
	return senders, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderID, error) {
	if r == nil || r.db == nil {
		return SenderID{}, errors.New("sender id repository is not configured")
	}
	return getSenderID(ctx, r.db, id, teamID, false)
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderID, error) {
	if r == nil || r.db == nil {
		return SenderID{}, errors.New("sender id repository is not configured")
	}
	sender, err := getSenderID(ctx, r.db, id, teamID, false)
	if err != nil {
		return SenderID{}, fmt.Errorf("get sender id for deletion: %w", err)
	}
	result, err := r.db.Exec(ctx, `DELETE FROM sender_ids WHERE id = $1 AND team_id = $2`, id, teamID)
	if err != nil {
		return SenderID{}, fmt.Errorf("delete sender id: %w", err)
	}
	if result.RowsAffected() != 1 {
		return SenderID{}, fmt.Errorf("delete sender id: %w", pgx.ErrNoRows)
	}
	return sender, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getSenderID(ctx context.Context, db queryRower, id, teamID uuid.UUID, lock bool) (SenderID, error) {
	query := `
		SELECT ` + senderIDProjection + `
		FROM sender_ids AS sender_id
		JOIN teams AS team ON team.id = sender_id.team_id
		WHERE sender_id.id = $1
		  AND sender_id.team_id = $2
		  AND team.status = 'active'`
	if lock {
		query += " FOR UPDATE OF sender_id"
	}
	sender, err := scanSenderID(db.QueryRow(ctx, query, id, teamID))
	if err != nil {
		return SenderID{}, fmt.Errorf("get sender id: %w", err)
	}
	return sender, nil
}

func scanSenderID(scanner rowScanner) (SenderID, error) {
	var id, teamID uuid.UUID
	var provider, rejectionReason *string
	var approvedAt, rejectedAt, suspendedAt, createdAt, updatedAt pgtype.Timestamptz
	var createdBy *uuid.UUID
	var name, countryCode, purpose, status string
	if err := scanner.Scan(
		&id,
		&teamID,
		&name,
		&countryCode,
		&purpose,
		&status,
		&provider,
		&rejectionReason,
		&approvedAt,
		&rejectedAt,
		&suspendedAt,
		&createdBy,
		&createdAt,
		&updatedAt,
	); err != nil {
		return SenderID{}, err
	}
	var createdByString *string
	if createdBy != nil {
		value := createdBy.String()
		createdByString = &value
	}
	return SenderID{
		ID:              id.String(),
		TeamID:          teamID.String(),
		Name:            name,
		CountryCode:     countryCode,
		Purpose:         purpose,
		Status:          status,
		Provider:        provider,
		RejectionReason: rejectionReason,
		ApprovedAt:      timestamptzPtr(approvedAt),
		RejectedAt:      timestamptzPtr(rejectedAt),
		SuspendedAt:     timestamptzPtr(suspendedAt),
		CreatedBy:       createdByString,
		CreatedAt:       createdAt.Time,
		UpdatedAt:       updatedAt.Time,
	}, nil
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
