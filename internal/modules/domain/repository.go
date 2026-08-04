package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

var ErrSenderDomainAlreadyExists = errors.New("sender domain already exists")

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db, queries: dbsqlc.New(db)} }

type ReconciliationClaim struct {
	Domain  SenderDomain
	Attempt int32
}

func (r *Repository) ClaimPendingReconciliations(ctx context.Context, workerID string, limit int32, staleBefore time.Time) ([]ReconciliationClaim, error) {
	rows, err := r.queries.ClaimSenderDomainsForReconciliation(ctx, dbsqlc.ClaimSenderDomainsForReconciliationParams{
		WorkerID: &workerID, BatchSize: limit, StaleBefore: pgtype.Timestamptz{Time: staleBefore, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("claim sender domain reconciliations: %w", err)
	}
	claims := make([]ReconciliationClaim, 0, len(rows))
	for _, row := range rows {
		claims = append(claims, ReconciliationClaim{Domain: senderDomainFromSQLC(row), Attempt: row.VerificationAttempts})
	}
	return claims, nil
}

func (r *Repository) CompleteReconciliation(ctx context.Context, id uuid.UUID, workerID, status string, records []VerificationRecord, nextCheckAt time.Time) (SenderDomain, error) {
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	row, err := r.queries.CompleteSenderDomainReconciliation(ctx, dbsqlc.CompleteSenderDomainReconciliationParams{
		ID: id, WorkerID: &workerID, Status: status, VerificationRecords: recordsJSON,
		NextCheckAt: pgtype.Timestamptz{Time: nextCheckAt, Valid: true},
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("complete sender domain reconciliation: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func (r *Repository) RecordReconciliationFailure(ctx context.Context, id uuid.UUID, workerID string, cause error, nextCheckAt time.Time) (SenderDomain, error) {
	reason := "sender domain reconciliation failed"
	if cause != nil {
		reason = cause.Error()
	}
	row, err := r.queries.RecordSenderDomainReconciliationFailure(ctx, dbsqlc.RecordSenderDomainReconciliationFailureParams{
		ID: id, WorkerID: &workerID, FailureReason: &reason,
		NextCheckAt: pgtype.Timestamptz{Time: nextCheckAt, Valid: true},
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("record sender domain reconciliation failure: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func (r *Repository) CompleteHealthCheck(ctx context.Context, id uuid.UUID, workerID string, nextCheckAt time.Time) (SenderDomain, error) {
	row, err := r.queries.CompleteSenderDomainHealthCheck(ctx, dbsqlc.CompleteSenderDomainHealthCheckParams{
		ID: id, WorkerID: &workerID, NextCheckAt: pgtype.Timestamptz{Time: nextCheckAt, Valid: true},
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("complete sender domain health check: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func (r *Repository) RecordHealthFailure(ctx context.Context, id uuid.UUID, workerID string, cause error, failureThreshold int32, nextCheckAt time.Time) (SenderDomain, error) {
	reason := "sender domain health check failed"
	if cause != nil {
		reason = cause.Error()
	}
	row, err := r.queries.RecordSenderDomainHealthFailure(ctx, dbsqlc.RecordSenderDomainHealthFailureParams{
		ID: id, WorkerID: &workerID, FailureReason: &reason, FailureThreshold: failureThreshold,
		NextCheckAt: pgtype.Timestamptz{Time: nextCheckAt, Valid: true},
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("record sender domain health failure: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func (r *Repository) Create(
	ctx context.Context,
	teamID uuid.UUID,
	domain string,
	provider string,
	providerRegion string,
	records []VerificationRecord,
	createdBy uuid.UUID,
) (SenderDomain, error) {
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	row, err := r.queries.CreateSenderDomain(ctx, dbsqlc.CreateSenderDomainParams{
		TeamID:              teamID,
		Domain:              domain,
		Provider:            provider,
		ProviderRegion:      providerRegion,
		VerificationRecords: recordsJSON,
		CreatedBy:           &createdBy,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return SenderDomain{}, ErrSenderDomainAlreadyExists
		}
		return SenderDomain{}, fmt.Errorf("create sender domain: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]SenderDomain, error) {
	rows, err := r.queries.ListSenderDomains(ctx, dbsqlc.ListSenderDomainsParams{TeamID: teamID})
	if err != nil {
		return nil, fmt.Errorf("list sender domains: %w", err)
	}
	domains := make([]SenderDomain, 0, len(rows))
	for _, row := range rows {
		domains = append(domains, senderDomainFromSQLC(row))
	}
	return domains, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderDomain, error) {
	row, err := r.queries.GetSenderDomain(ctx, dbsqlc.GetSenderDomainParams{ID: id, TeamID: teamID})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("get sender domain: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func (r *Repository) UpdateVerification(ctx context.Context, id, teamID uuid.UUID, status string, records []VerificationRecord, failureReason *string) (SenderDomain, error) {
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	row, err := r.queries.UpdateSenderDomainVerification(ctx, dbsqlc.UpdateSenderDomainVerificationParams{
		Status: status, VerificationRecords: recordsJSON, FailureReason: failureReason, ID: id, TeamID: teamID,
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("update sender domain verification: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func (r *Repository) UpdateManualHealthCheck(ctx context.Context, id, teamID uuid.UUID, records []VerificationRecord, failureReason *string) (SenderDomain, error) {
	if r == nil || r.db == nil {
		return SenderDomain{}, errors.New("sender domain repository is not configured")
	}
	recordsJSON, err := json.Marshal(records)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("marshal sender domain verification records: %w", err)
	}
	commandTag, err := r.db.Exec(ctx, `
		UPDATE sender_domains
		SET verification_records = $3,
			health_status = CASE
				WHEN $4::text IS NULL THEN 'healthy'
				WHEN consecutive_health_failures + 1 >= $5 THEN 'degraded'
				ELSE health_status
			END,
			consecutive_health_failures = CASE
				WHEN $4::text IS NULL THEN 0
				ELSE consecutive_health_failures + 1
			END,
			failure_reason = $4,
			last_checked_at = now(),
			last_health_checked_at = now(),
			last_health_failure_at = CASE WHEN $4::text IS NULL THEN last_health_failure_at ELSE now() END,
			updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'verified'
	`, id, teamID, recordsJSON, failureReason, DefaultHealthFailureThreshold)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("update manual sender domain health check: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return SenderDomain{}, errors.New("verified sender domain not found")
	}
	return r.Get(ctx, id, teamID)
}

// Disable marks a sender domain unusable before its provider identity is removed.
func (r *Repository) Disable(ctx context.Context, id, teamID uuid.UUID) (SenderDomain, error) {
	if r == nil || r.db == nil {
		return SenderDomain{}, errors.New("sender domain repository is not configured")
	}
	commandTag, err := r.db.Exec(ctx, `
		UPDATE sender_domains
		SET status = 'disabled',
			disabled_at = COALESCE(disabled_at, now()),
			next_check_at = now(),
			reconcile_locked_at = NULL,
			reconcile_locked_by = NULL,
			updated_at = now()
		WHERE id = $1 AND team_id = $2
	`, id, teamID)
	if err != nil {
		return SenderDomain{}, fmt.Errorf("disable sender domain: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return SenderDomain{}, pgx.ErrNoRows
	}
	return r.Get(ctx, id, teamID)
}

// PurgeIfUnreferenced removes a disabled domain only when no email message still references it.
func (r *Repository) PurgeIfUnreferenced(ctx context.Context, id, teamID uuid.UUID) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("sender domain repository is not configured")
	}
	commandTag, err := r.db.Exec(ctx, `
		DELETE FROM sender_domains AS domain
		WHERE domain.id = $1
		  AND domain.team_id = $2
		  AND domain.status = 'disabled'
		  AND NOT EXISTS (
			SELECT 1
			FROM email_messages AS message
			WHERE message.sender_domain_id = domain.id
		  )
	`, id, teamID)
	if err != nil {
		return false, fmt.Errorf("purge sender domain: %w", err)
	}
	return commandTag.RowsAffected() > 0, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderDomain, error) {
	row, err := r.queries.DeleteSenderDomain(ctx, dbsqlc.DeleteSenderDomainParams{ID: id, TeamID: teamID})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("delete sender domain: %w", err)
	}
	return senderDomainFromSQLC(row), nil
}

func senderDomainFromSQLC(row dbsqlc.SenderDomain) SenderDomain {
	var createdBy *string
	if row.CreatedBy != nil {
		value := row.CreatedBy.String()
		createdBy = &value
	}
	var records []VerificationRecord
	if err := json.Unmarshal(row.VerificationRecords, &records); err != nil {
		records = []VerificationRecord{}
	}
	return SenderDomain{
		ID: row.ID.String(), TeamID: row.TeamID.String(), Domain: row.Domain,
		Provider: row.Provider, ProviderRegion: row.ProviderRegion, Status: row.Status,
		VerificationRecords: records, FailureReason: row.FailureReason,
		HealthStatus: row.HealthStatus, ConsecutiveHealthFailures: row.ConsecutiveHealthFailures,
		LastCheckedAt:       pgconv.TimestamptzToTimePtr(row.LastCheckedAt),
		LastHealthCheckedAt: pgconv.TimestamptzToTimePtr(row.LastHealthCheckedAt),
		LastHealthFailureAt: pgconv.TimestamptzToTimePtr(row.LastHealthFailureAt),
		VerifiedAt:          pgconv.TimestamptzToTimePtr(row.VerifiedAt),
		DisabledAt:          pgconv.TimestamptzToTimePtr(row.DisabledAt), CreatedBy: createdBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
