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
	binding.id,
	asset.team_id,
	asset.identity,
	COALESCE(binding.country_code::text, ''),
	COALESCE(asset.purpose, ''),
	CASE binding.status
		WHEN 'active' THEN 'approved'
		WHEN 'disabled' THEN 'inactive'
		WHEN 'failed' THEN 'rejected'
		ELSE binding.status
	END,
	binding.provider,
	binding.rejection_reason,
	binding.verified_at,
	binding.rejected_at,
	binding.suspended_at,
	asset.created_by,
	binding.created_at,
	binding.updated_at`

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

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SenderID{}, fmt.Errorf("begin sender id creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var assetID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO sender_assets (
			owner_type, team_id, channel, identity, normalized_identity,
			purpose, status, health_status, created_by
		)
		SELECT 'team', team.id, 'sms', $2, $3, NULLIF(trim($4), ''),
			'pending', 'unknown', $5
		FROM teams AS team
		WHERE team.id = $1
		  AND team.status = 'active'
		ON CONFLICT (team_id, channel, normalized_identity)
			WHERE owner_type = 'team'
		DO UPDATE SET
			identity = EXCLUDED.identity,
			purpose = EXCLUDED.purpose,
			updated_at = now()
		RETURNING id
	`, teamID, strings.TrimSpace(name), normalizedName, purpose, createdBy).Scan(&assetID)
	if err != nil {
		return SenderID{}, fmt.Errorf("create sender asset: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO sender_asset_grants (
			team_id, sender_asset_id, channel, status, granted_by
		) VALUES ($1, $2, 'sms', 'active', $3)
		ON CONFLICT (team_id, sender_asset_id)
		DO UPDATE SET
			status = 'active',
			revoked_at = NULL,
			updated_at = now()
	`, teamID, assetID, createdBy); err != nil {
		return SenderID{}, fmt.Errorf("grant sender asset: %w", err)
	}

	var bindingID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO sender_provider_bindings (
			sender_asset_id, provider, country_code, status, health_status
		) VALUES ($1, $2, $3, 'pending', 'unknown')
		RETURNING id
	`, assetID, normalizedProvider, countryCode).Scan(&bindingID)
	if err != nil {
		if isUniqueViolation(err) {
			return SenderID{}, ErrSenderIDAlreadyExists
		}
		return SenderID{}, fmt.Errorf("create sender provider binding: %w", err)
	}

	sender, err := getSenderID(ctx, tx, bindingID, teamID, false)
	if err != nil {
		return SenderID{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderID{}, fmt.Errorf("commit sender id creation: %w", err)
	}
	return sender, nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]SenderID, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("sender id repository is not configured")
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+senderIDProjection+`
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		JOIN sender_asset_grants AS grant_record
		  ON grant_record.sender_asset_id = asset.id
		 AND grant_record.team_id = $1
		 AND grant_record.channel = 'sms'
		 AND grant_record.status = 'active'
		JOIN teams AS team ON team.id = grant_record.team_id
		WHERE asset.channel = 'sms'
		  AND team.status = 'active'
		ORDER BY binding.created_at DESC
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
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return SenderID{}, fmt.Errorf("begin sender id deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	sender, err := getSenderID(ctx, tx, id, teamID, true)
	if err != nil {
		return SenderID{}, fmt.Errorf("get sender id for deletion: %w", err)
	}
	var assetID uuid.UUID
	if err := tx.QueryRow(ctx, `
		DELETE FROM sender_provider_bindings AS binding
		USING sender_assets AS asset
		WHERE binding.id = $1
		  AND binding.sender_asset_id = asset.id
		  AND asset.team_id = $2
		  AND asset.channel = 'sms'
		RETURNING binding.sender_asset_id
	`, id, teamID).Scan(&assetID); err != nil {
		return SenderID{}, fmt.Errorf("delete sender provider binding: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM sender_assets AS asset
		WHERE asset.id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM sender_provider_bindings AS binding
			WHERE binding.sender_asset_id = asset.id
		  )
	`, assetID); err != nil {
		return SenderID{}, fmt.Errorf("delete unbound sender asset: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderID{}, fmt.Errorf("commit sender id deletion: %w", err)
	}
	return sender, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getSenderID(ctx context.Context, db queryRower, id, teamID uuid.UUID, lock bool) (SenderID, error) {
	query := `
		SELECT ` + senderIDProjection + `
		FROM sender_provider_bindings AS binding
		JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
		JOIN sender_asset_grants AS grant_record
		  ON grant_record.sender_asset_id = asset.id
		 AND grant_record.team_id = $2
		 AND grant_record.channel = 'sms'
		 AND grant_record.status = 'active'
		JOIN teams AS team ON team.id = grant_record.team_id
		WHERE binding.id = $1
		  AND asset.channel = 'sms'
		  AND team.status = 'active'`
	if lock {
		query += " FOR UPDATE OF binding, asset"
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
