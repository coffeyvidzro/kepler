package senderidreconciliation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
			SELECT sender_id.id
			FROM sender_ids AS sender_id
			WHERE sender_id.country_code = 'GH'
			  AND lower(sender_id.provider) = $1
			  AND sender_id.status = 'pending'
			  AND sender_id.next_check_at <= now()
			  AND (
				sender_id.reconcile_locked_at IS NULL
				OR sender_id.reconcile_locked_at < $4
			  )
			ORDER BY sender_id.next_check_at, sender_id.created_at, sender_id.id
			FOR UPDATE OF sender_id SKIP LOCKED
			LIMIT $3
		), updated AS (
			UPDATE sender_ids AS sender_id
			SET reconcile_locked_at = now(),
				reconcile_locked_by = $2,
				reconciliation_attempts = sender_id.reconciliation_attempts + 1,
				updated_at = now()
			FROM candidates
			WHERE sender_id.id = candidates.id
			RETURNING sender_id.*
		)
		SELECT sender_id.id, sender_id.name, sender_id.country_code::text,
			COALESCE(sender_id.provider, ''), COALESCE(sender_id.provider_status, ''),
			sender_id.submitted_at, sender_id.reconciliation_attempts
		FROM updated AS sender_id
		ORDER BY sender_id.next_check_at, sender_id.created_at, sender_id.id
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
		UPDATE sender_ids
		SET provider_status = $3,
			submitted_at = COALESCE(submitted_at, now()),
			last_checked_at = now(),
			next_check_at = $4,
			reconciliation_attempts = 0,
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
	result, err := repository.db.Exec(ctx, `
		UPDATE sender_ids
		SET status = $3,
			provider_status = $4,
			provider_whitelisted = $5,
			health_status = CASE
				WHEN $3 = 'approved' AND $5 THEN 'healthy'
				WHEN $3 IN ('rejected', 'suspended') THEN 'degraded'
				ELSE health_status
			END,
			submitted_at = COALESCE(submitted_at, now()),
			last_checked_at = now(),
			next_check_at = $7,
			reconciliation_attempts = 0,
			last_error = NULL,
			rejection_reason = CASE WHEN $3 = 'rejected' THEN $6 ELSE NULL END,
			approved_at = CASE
				WHEN $3 = 'approved' THEN COALESCE(approved_at, now())
				ELSE approved_at
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
	`, id, strings.TrimSpace(workerID), status, strings.TrimSpace(providerStatus), whitelisted, rejectionReason, nextCheckAt)
	if err != nil {
		return fmt.Errorf("complete Sender ID provider status: %w", err)
	}

	if result.RowsAffected() != 1 {
		return ErrRegistrationClaimLost
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
		UPDATE sender_ids
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
