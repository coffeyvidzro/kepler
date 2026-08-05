from pathlib import Path

sender_repository = Path("internal/modules/senderid/repository.go")
source = sender_repository.read_text()
source = source.replace('"strings"\n', '"strings"\n\t"time"\n', 1)
sender_repository.write_text(source)

outbound = Path("internal/delivery/email/outbound/repository.go")
source = outbound.read_text()
old = '''\t\t\tmessage.sender_provider_binding_id IS NULL OR EXISTS (
\t\t\t\tSELECT 1
\t\t\t\tFROM sender_domains AS domain
\t\t\t\tWHERE domain.id = message.sender_provider_binding_id
\t\t\t\t  AND domain.team_id = message.team_id
\t\t\t\t  AND domain.status = 'verified'
\t\t\t\t  AND domain.disabled_at IS NULL
\t\t\t\t  AND domain.health_status <> 'degraded'
\t\t\t) AS authorized'''
new = '''\t\t\tmessage.sender_provider_binding_id IS NULL OR EXISTS (
\t\t\t\tSELECT 1
\t\t\t\tFROM sender_provider_bindings AS binding
\t\t\t\tJOIN sender_assets AS asset
\t\t\t\t  ON asset.id = binding.sender_asset_id
\t\t\t\tJOIN sender_asset_grants AS grant_record
\t\t\t\t  ON grant_record.sender_asset_id = asset.id
\t\t\t\t AND grant_record.team_id = message.team_id
\t\t\t\t AND grant_record.channel = 'email'
\t\t\t\t AND grant_record.status = 'active'
\t\t\t\tWHERE binding.id = message.sender_provider_binding_id
\t\t\t\t  AND asset.channel = 'email'
\t\t\t\t  AND binding.status = 'active'
\t\t\t\t  AND binding.verified
\t\t\t\t  AND binding.disabled_at IS NULL
\t\t\t\t  AND binding.health_status <> 'degraded'
\t\t\t) AS authorized'''
if source.count(old) != 1:
    raise SystemExit("email delivery authorization query was not found")
outbound.write_text(source.replace(old, new, 1))

Path("internal/delivery/senderid/repository.go").write_text(r'''package senderidreconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RegistrationClaim struct {
	ID                  uuid.UUID
	Name                string
	CountryCode         string
	Provider            string
	ProviderStatus      string
	ProviderSubmittedAt *time.Time
	Attempt             int32
}

type registrationRepository interface {
	ClaimPendingRegistrations(context.Context, string, string, int32, time.Time) ([]RegistrationClaim, error)
	CompleteSubmission(context.Context, uuid.UUID, string, string, time.Time) error
	CompleteStatus(context.Context, uuid.UUID, string, string, string, bool, *string, time.Time) error
	RecordProviderFailure(context.Context, uuid.UUID, string, string, error, time.Time) error
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) ClaimPendingRegistrations(
	ctx context.Context,
	workerID string,
	providerID string,
	limit int32,
	staleBefore time.Time,
) ([]RegistrationClaim, error) {
	if repository == nil || repository.db == nil {
		return nil, errors.New("sender ID repository is not configured")
	}
	workerID = strings.TrimSpace(workerID)
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	if workerID == "" || providerID == "" {
		return nil, errors.New("sender ID reconciliation requires worker and provider IDs")
	}
	if limit <= 0 {
		limit = 25
	}

	rows, err := repository.db.Query(ctx, `
		WITH candidates AS (
			SELECT binding.id
			FROM sender_provider_bindings AS binding
			JOIN sender_assets AS asset
			  ON asset.id = binding.sender_asset_id
			WHERE asset.channel = 'sms'
			  AND asset.owner_type = 'team'
			  AND binding.country_code = 'GH'
			  AND lower(binding.provider) = $1
			  AND binding.status = 'pending'
			  AND binding.next_check_at <= now()
			  AND (
				binding.reconcile_locked_at IS NULL
				OR binding.reconcile_locked_at < $4
			  )
			ORDER BY binding.next_check_at, binding.created_at, binding.id
			FOR UPDATE OF binding SKIP LOCKED
			LIMIT $3
		), updated AS (
			UPDATE sender_provider_bindings AS binding
			SET reconcile_locked_at = now(),
				reconcile_locked_by = $2,
				attempts = binding.attempts + 1,
				updated_at = now()
			FROM candidates
			WHERE binding.id = candidates.id
			RETURNING binding.*
		)
		SELECT binding.id,
			asset.identity,
			COALESCE(binding.country_code::text, ''),
			COALESCE(binding.provider, ''),
			COALESCE(binding.provider_status, ''),
			binding.submitted_at,
			binding.attempts
		FROM updated AS binding
		JOIN sender_assets AS asset
		  ON asset.id = binding.sender_asset_id
		ORDER BY binding.next_check_at, binding.created_at, binding.id
	`, providerID, workerID, limit, staleBefore)
	if err != nil {
		return nil, fmt.Errorf("claim pending Sender ID registrations: %w", err)
	}
	defer rows.Close()

	claims := make([]RegistrationClaim, 0, limit)
	for rows.Next() {
		var claim RegistrationClaim
		var submittedAt pgtype.Timestamptz
		if err := rows.Scan(
			&claim.ID,
			&claim.Name,
			&claim.CountryCode,
			&claim.Provider,
			&claim.ProviderStatus,
			&submittedAt,
			&claim.Attempt,
		); err != nil {
			return nil, fmt.Errorf("scan Sender ID registration claim: %w", err)
		}
		if submittedAt.Valid {
			value := submittedAt.Time
			claim.ProviderSubmittedAt = &value
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Sender ID registration claims: %w", err)
	}
	return claims, nil
}

func (repository *Repository) CompleteSubmission(
	ctx context.Context,
	id uuid.UUID,
	workerID string,
	providerStatus string,
	nextCheckAt time.Time,
) error {
	if repository == nil || repository.db == nil {
		return errors.New("sender ID repository is not configured")
	}
	result, err := repository.db.Exec(ctx, `
		UPDATE sender_provider_bindings
		SET provider_status = $3,
			submitted_at = COALESCE(submitted_at, now()),
			last_checked_at = now(),
			next_check_at = $4,
			attempts = 0,
			last_error = NULL,
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND reconcile_locked_by = $2
	`, id, strings.TrimSpace(workerID), strings.TrimSpace(providerStatus), nextCheckAt)
	if err != nil {
		return fmt.Errorf("complete Sender ID registration claim: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrRegistrationClaimLost
	}
	return nil
}

func (repository *Repository) CompleteStatus(
	ctx context.Context,
	id uuid.UUID,
	workerID string,
	status string,
	providerStatus string,
	whitelisted bool,
	rejectionReason *string,
	nextCheckAt time.Time,
) error {
	if repository == nil || repository.db == nil {
		return errors.New("sender ID repository is not configured")
	}
	status = strings.ToLower(strings.TrimSpace(status))
	tx, err := repository.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Sender ID status completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var assetID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE sender_provider_bindings
		SET status = CASE $3
				WHEN 'approved' THEN 'active'
				WHEN 'inactive' THEN 'disabled'
				ELSE $3
			END,
			provider_status = $4,
			verified = $3 = 'approved',
			provider_whitelisted = $5,
			health_status = CASE
				WHEN $3 = 'approved' AND $5 THEN 'healthy'
				WHEN $3 IN ('rejected', 'suspended') THEN 'degraded'
				ELSE health_status
			END,
			submitted_at = COALESCE(submitted_at, now()),
			last_checked_at = now(),
			next_check_at = $7,
			attempts = 0,
			last_error = NULL,
			rejection_reason = CASE WHEN $3 = 'rejected' THEN $6 ELSE NULL END,
			verified_at = CASE
				WHEN $3 = 'approved' THEN COALESCE(verified_at, now())
				ELSE verified_at
			END,
			rejected_at = CASE
				WHEN $3 = 'rejected' THEN COALESCE(rejected_at, now())
				ELSE rejected_at
			END,
			suspended_at = CASE
				WHEN $3 = 'suspended' THEN COALESCE(suspended_at, now())
				ELSE suspended_at
			END,
			disabled_at = CASE
				WHEN $3 = 'inactive' THEN COALESCE(disabled_at, now())
				ELSE disabled_at
			END,
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND reconcile_locked_by = $2
		RETURNING sender_asset_id
	`, id, strings.TrimSpace(workerID), status, strings.TrimSpace(providerStatus), whitelisted, rejectionReason, nextCheckAt).Scan(&assetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrRegistrationClaimLost
	}
	if err != nil {
		return fmt.Errorf("complete Sender ID provider status: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sender_assets
		SET status = CASE $2
				WHEN 'approved' THEN 'active'
				WHEN 'rejected' THEN 'failed'
				WHEN 'inactive' THEN 'disabled'
				ELSE $2
			END,
			health_status = CASE
				WHEN $2 = 'approved' AND $3 THEN 'healthy'
				WHEN $2 IN ('rejected', 'suspended') THEN 'degraded'
				ELSE health_status
			END,
			updated_at = now()
		WHERE id = $1
	`, assetID, status, whitelisted); err != nil {
		return fmt.Errorf("update Sender ID asset status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Sender ID status completion: %w", err)
	}
	return nil
}

func (repository *Repository) RecordProviderFailure(
	ctx context.Context,
	id uuid.UUID,
	workerID string,
	providerStatus string,
	providerError error,
	nextCheckAt time.Time,
) error {
	if repository == nil || repository.db == nil {
		return errors.New("sender ID repository is not configured")
	}
	message := "sender ID provider operation failed"
	if providerError != nil {
		message = providerError.Error()
	}
	result, err := repository.db.Exec(ctx, `
		UPDATE sender_provider_bindings
		SET provider_status = COALESCE(NULLIF($3, ''), provider_status),
			last_checked_at = now(),
			next_check_at = $5,
			last_error = $4,
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND reconcile_locked_by = $2
	`, id, strings.TrimSpace(workerID), strings.TrimSpace(providerStatus), message, nextCheckAt)
	if err != nil {
		return fmt.Errorf("record Sender ID provider failure: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrRegistrationClaimLost
	}
	return nil
}
''')
