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
			SELECT id
			FROM sender_ids
			WHERE country_code = 'GH'
			  AND lower(provider) = $1
			  AND status = 'pending'
			  AND next_status_check_at <= now()
			  AND (
				registration_locked_at IS NULL
				OR registration_locked_at < $4
			  )
			ORDER BY next_status_check_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE sender_ids AS sender
		SET registration_locked_at = now(),
			registration_locked_by = $2,
			provider_attempts = sender.provider_attempts + 1,
			updated_at = now()
		FROM candidates
		WHERE sender.id = candidates.id
		RETURNING sender.id,
			sender.name,
			sender.country_code,
			sender.provider,
			COALESCE(sender.provider_status, ''),
			sender.provider_submitted_at,
			sender.provider_attempts
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
	return repository.completeClaim(ctx, id, workerID, `
		UPDATE sender_ids
		SET provider_status = $3,
			provider_submitted_at = COALESCE(provider_submitted_at, now()),
			provider_last_checked_at = now(),
			next_status_check_at = $4,
			provider_attempts = 0,
			provider_error = NULL,
			registration_locked_at = NULL,
			registration_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND registration_locked_by = $2
	`, strings.TrimSpace(providerStatus), nextCheckAt)
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
	result, err := repository.db.Exec(ctx, `
		UPDATE sender_ids
		SET status = $3,
			provider_status = $4,
			provider_whitelisted = $5,
			provider_submitted_at = COALESCE(provider_submitted_at, now()),
			provider_last_checked_at = now(),
			next_status_check_at = $7,
			provider_attempts = 0,
			provider_error = NULL,
			rejection_reason = CASE WHEN $3 = 'rejected' THEN $6 ELSE NULL END,
			approved_at = CASE
				WHEN $3 = 'approved' THEN COALESCE(approved_at, now())
				ELSE approved_at
			END,
			rejected_at = CASE
				WHEN $3 = 'rejected' THEN COALESCE(rejected_at, now())
				ELSE rejected_at
			END,
			registration_locked_at = NULL,
			registration_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND registration_locked_by = $2
	`, id, strings.TrimSpace(workerID), strings.ToLower(strings.TrimSpace(status)),
		strings.TrimSpace(providerStatus), whitelisted, rejectionReason, nextCheckAt)
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
	message := "Sender ID provider operation failed"
	if providerError != nil {
		message = providerError.Error()
	}
	result, err := repository.db.Exec(ctx, `
		UPDATE sender_ids
		SET provider_status = COALESCE(NULLIF($3, ''), provider_status),
			provider_last_checked_at = now(),
			next_status_check_at = $5,
			provider_error = $4,
			registration_locked_at = NULL,
			registration_locked_by = NULL,
			updated_at = now()
		WHERE id = $1
		  AND registration_locked_by = $2
	`, id, strings.TrimSpace(workerID), strings.TrimSpace(providerStatus), message, nextCheckAt)
	if err != nil {
		return fmt.Errorf("record Sender ID provider failure: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrRegistrationClaimLost
	}
	return nil
}

func (repository *Repository) completeClaim(
	ctx context.Context,
	id uuid.UUID,
	workerID string,
	query string,
	value any,
	nextCheckAt time.Time,
) error {
	if repository == nil || repository.db == nil {
		return errors.New("sender ID repository is not configured")
	}
	result, err := repository.db.Exec(ctx, query, id, strings.TrimSpace(workerID), value, nextCheckAt)
	if err != nil {
		return fmt.Errorf("complete Sender ID registration claim: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrRegistrationClaimLost
	}
	return nil
}
