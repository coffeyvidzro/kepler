package domain

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

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
)

var ErrSenderDomainAlreadyExists = errors.New("sender domain already exists")

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

type CreateDomainInput struct {
	TeamID              uuid.UUID
	Name                string
	Provider            string
	ProviderAccount     string
	ProviderRegion      string
	CustomReturnPath    string
	CreatedBy           uuid.UUID
	Configuration       DomainConfiguration
	VerificationRecords []VerificationRecord
}

type ReconciliationClaim struct {
	Domain  SenderDomain
	Attempt int32
}

func (r *Repository) Create(ctx context.Context, input CreateDomainInput) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("begin sender domain creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	row, err := queries.CreateDomain(ctx, dbsqlc.CreateDomainParams{
		TeamID:            input.TeamID,
		Name:              strings.ToLower(strings.TrimSpace(input.Name)),
		Provider:          strings.ToLower(strings.TrimSpace(input.Provider)),
		ProviderAccount:   strings.ToLower(strings.TrimSpace(input.ProviderAccount)),
		ProviderRegion:    strings.ToLower(strings.TrimSpace(input.ProviderRegion)),
		CustomReturnPath:  strings.ToLower(strings.TrimSpace(input.CustomReturnPath)),
		OpenTracking:      input.Configuration.OpenTracking,
		ClickTracking:     input.Configuration.ClickTracking,
		TrackingSubdomain: input.Configuration.TrackingSubdomain,
		TlsMode:           input.Configuration.TLS,
		SendingEnabled:    input.Configuration.Capabilities.Sending,
		ReceivingEnabled:  input.Configuration.Capabilities.Receiving,
		CreatedBy:         &input.CreatedBy,
	})
	if isUniqueViolation(err) {
		return SenderDomain{}, ErrSenderDomainAlreadyExists
	}
	if err != nil {
		return SenderDomain{}, fmt.Errorf("create sender domain: %w", err)
	}
	if err := replaceDomainDNSRecords(ctx, queries, row.ID, input.VerificationRecords); err != nil {
		return SenderDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, fmt.Errorf("commit sender domain creation: %w", err)
	}
	return r.Get(ctx, row.ID, input.TeamID)
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID) ([]SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ListDomains(ctx, dbsqlc.ListDomainsParams{TeamID: teamID})
	if err != nil {
		return nil, fmt.Errorf("list sender domains: %w", err)
	}
	values := make([]SenderDomain, 0, len(rows))
	for _, row := range rows {
		value, err := r.domainWithRecords(ctx, row)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	row, err := r.queries.GetDomain(ctx, dbsqlc.GetDomainParams{ID: id, TeamID: teamID})
	if err != nil {
		return SenderDomain{}, err
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) GetByName(ctx context.Context, teamID uuid.UUID, name string) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	row, err := r.queries.GetDomainByName(ctx, dbsqlc.GetDomainByNameParams{
		Name: name, TeamID: teamID,
	})
	if err != nil {
		return SenderDomain{}, err
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) getByID(ctx context.Context, id uuid.UUID) (SenderDomain, error) {
	row, err := r.queries.GetDomainByID(ctx, dbsqlc.GetDomainByIDParams{ID: id})
	if err != nil {
		return SenderDomain{}, err
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) UpdateConfiguration(
	ctx context.Context,
	id, teamID uuid.UUID,
	configuration DomainConfiguration,
) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	row, err := r.queries.UpdateDomainConfiguration(ctx, dbsqlc.UpdateDomainConfigurationParams{
		OpenTracking:      configuration.OpenTracking,
		ClickTracking:     configuration.ClickTracking,
		TrackingSubdomain: configuration.TrackingSubdomain,
		TlsMode:           configuration.TLS,
		SendingEnabled:    configuration.Capabilities.Sending,
		ReceivingEnabled:  configuration.Capabilities.Receiving,
		ID:                id,
		TeamID:            teamID,
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("update sender domain configuration: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) UpdateVerification(
	ctx context.Context,
	id, teamID uuid.UUID,
	status string,
	records []VerificationRecord,
	failureReason *string,
) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("begin sender domain verification update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	row, err := queries.UpdateDomainVerification(ctx, dbsqlc.UpdateDomainVerificationParams{
		Status: status, FailureReason: failureReason, ID: id, TeamID: teamID,
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("update sender domain verification: %w", err)
	}
	if err := replaceDomainDNSRecords(ctx, queries, id, records); err != nil {
		return SenderDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, fmt.Errorf("commit sender domain verification: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) ClaimPendingReconciliations(
	ctx context.Context,
	workerID string,
	limit int32,
	staleBefore time.Time,
) ([]ReconciliationClaim, error) {
	if err := r.requireConfigured(); err != nil {
		return nil, err
	}
	rows, err := r.queries.ClaimPendingDomainReconciliations(ctx, dbsqlc.ClaimPendingDomainReconciliationsParams{
		StaleBefore: timestamptz(staleBefore),
		ClaimLimit:  limit,
		WorkerID:    strings.TrimSpace(workerID),
	})
	if err != nil {
		return nil, fmt.Errorf("claim sender domain reconciliations: %w", err)
	}
	claims := make([]ReconciliationClaim, 0, len(rows))
	for _, row := range rows {
		value, err := r.domainWithRecords(ctx, domainFromReconciliationRow(row))
		if err != nil {
			return nil, err
		}
		claims = append(claims, ReconciliationClaim{
			Domain: value, Attempt: row.ReconciliationAttempts,
		})
	}
	return claims, nil
}

func (r *Repository) CompleteReconciliation(
	ctx context.Context,
	id uuid.UUID,
	workerID, status string,
	records []VerificationRecord,
	nextCheckAt time.Time,
) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("begin sender domain reconciliation completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	row, err := queries.CompleteDomainReconciliation(ctx, dbsqlc.CompleteDomainReconciliationParams{
		Status:        status,
		FailureReason: nil,
		NextCheckAt:   timestamptz(nextCheckAt),
		ID:            id,
		WorkerID:      strings.TrimSpace(workerID),
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("complete sender domain reconciliation: %w", err)
	}
	if err := replaceDomainDNSRecords(ctx, queries, id, records); err != nil {
		return SenderDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, fmt.Errorf("commit sender domain reconciliation completion: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) RecordReconciliationFailure(
	ctx context.Context,
	id uuid.UUID,
	workerID string,
	cause error,
	nextCheckAt time.Time,
) (SenderDomain, error) {
	reason := "sender domain reconciliation failed"
	if cause != nil {
		reason = cause.Error()
	}
	row, err := r.queries.RecordDomainReconciliationFailure(ctx, dbsqlc.RecordDomainReconciliationFailureParams{
		LastError:   &reason,
		NextCheckAt: timestamptz(nextCheckAt),
		ID:          id,
		WorkerID:    strings.TrimSpace(workerID),
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("record sender domain reconciliation failure: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) CompleteHealthCheck(
	ctx context.Context,
	id uuid.UUID,
	workerID string,
	nextCheckAt time.Time,
) (SenderDomain, error) {
	row, err := r.queries.CompleteDomainHealthCheck(ctx, dbsqlc.CompleteDomainHealthCheckParams{
		NextCheckAt: timestamptz(nextCheckAt),
		ID:          id,
		WorkerID:    strings.TrimSpace(workerID),
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("complete sender domain health check: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) RecordHealthFailure(
	ctx context.Context,
	id uuid.UUID,
	workerID string,
	cause error,
	failureThreshold int32,
	nextCheckAt time.Time,
) (SenderDomain, error) {
	reason := "sender domain health check failed"
	if cause != nil {
		reason = cause.Error()
	}
	row, err := r.queries.RecordDomainHealthFailure(ctx, dbsqlc.RecordDomainHealthFailureParams{
		FailureThreshold: failureThreshold,
		LastError:        &reason,
		NextCheckAt:      timestamptz(nextCheckAt),
		ID:               id,
		WorkerID:         strings.TrimSpace(workerID),
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("record sender domain health failure: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) UpdateManualHealthCheck(
	ctx context.Context,
	id, teamID uuid.UUID,
	records []VerificationRecord,
	failureReason *string,
) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("begin sender domain manual health update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	row, err := queries.UpdateDomainManualHealthCheck(ctx, dbsqlc.UpdateDomainManualHealthCheckParams{
		FailureReason:    failureReason,
		FailureThreshold: DefaultHealthFailureThreshold,
		ID:               id,
		TeamID:           teamID,
	})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("update manual sender domain health check: %w", err)
	}
	if err := replaceDomainDNSRecords(ctx, queries, id, records); err != nil {
		return SenderDomain{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, fmt.Errorf("commit sender domain manual health update: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) Disable(ctx context.Context, id, teamID uuid.UUID) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	row, err := r.queries.DisableDomain(ctx, dbsqlc.DisableDomainParams{ID: id, TeamID: teamID})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("disable sender domain: %w", err)
	}
	return r.domainWithRecords(ctx, row)
}

func (r *Repository) Delete(ctx context.Context, id, teamID uuid.UUID) (SenderDomain, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, err
	}
	row, err := r.queries.DeleteDomain(ctx, dbsqlc.DeleteDomainParams{ID: id, TeamID: teamID})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("delete sender domain: %w", err)
	}
	return domainFromSQLC(row, nil), nil
}

func (r *Repository) PurgeIfUnreferenced(ctx context.Context, id, teamID uuid.UUID) (bool, error) {
	if err := r.requireConfigured(); err != nil {
		return false, err
	}
	_, err := r.queries.DeleteDisabledDomainIfUnreferenced(ctx, dbsqlc.DeleteDisabledDomainIfUnreferencedParams{
		ID: id, TeamID: teamID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("purge sender domain: %w", err)
	}
	return true, nil
}

func (r *Repository) domainWithRecords(ctx context.Context, row dbsqlc.Domain) (SenderDomain, error) {
	records, err := r.queries.ListDomainDNSRecords(ctx, dbsqlc.ListDomainDNSRecordsParams{DomainID: row.ID})
	if err != nil {
		return SenderDomain{}, fmt.Errorf("list sender domain DNS records: %w", err)
	}
	return domainFromSQLC(row, records), nil
}

func replaceDomainDNSRecords(
	ctx context.Context,
	queries *dbsqlc.Queries,
	domainID uuid.UUID,
	records []VerificationRecord,
) error {
	if err := queries.DeleteCurrentDomainDNSRecords(ctx, dbsqlc.DeleteCurrentDomainDNSRecordsParams{DomainID: domainID}); err != nil {
		return fmt.Errorf("delete current sender domain DNS records: %w", err)
	}
	for _, record := range records {
		var priority *int32
		if record.Priority != nil {
			value := int32(*record.Priority)
			priority = &value
		}
		if _, err := queries.CreateDomainDNSRecord(ctx, dbsqlc.CreateDomainDNSRecordParams{
			DomainID: domainID,
			Purpose:  verificationRecordPurpose(record),
			Record:   record.Record,
			Name:     record.Name,
			Type:     record.Type,
			Value:    record.Value,
			Ttl:      record.TTL,
			Priority: priority,
			Status:   record.Status,
		}); err != nil {
			return fmt.Errorf("create sender domain DNS record: %w", err)
		}
	}
	return nil
}

func domainFromReconciliationRow(row dbsqlc.ClaimPendingDomainReconciliationsRow) dbsqlc.Domain {
	return dbsqlc.Domain{
		ID: row.ID, TeamID: row.TeamID, Name: row.Name, NormalizedName: row.NormalizedName,
		Provider: row.Provider, ProviderAccount: row.ProviderAccount, ProviderRegion: row.ProviderRegion,
		ProviderExternalID: row.ProviderExternalID, Status: row.Status, ProviderStatus: row.ProviderStatus,
		OpenTracking: row.OpenTracking, ClickTracking: row.ClickTracking,
		TrackingSubdomain: row.TrackingSubdomain, ActiveTrackingSubdomain: row.ActiveTrackingSubdomain,
		TlsMode: row.TlsMode, SendingEnabled: row.SendingEnabled, ReceivingEnabled: row.ReceivingEnabled,
		CustomReturnPath: row.CustomReturnPath, HealthStatus: row.HealthStatus,
		ConsecutiveHealthFailures: row.ConsecutiveHealthFailures, FailureReason: row.FailureReason,
		LastError: row.LastError, SubmittedAt: row.SubmittedAt, VerifiedAt: row.VerifiedAt,
		DisabledAt: row.DisabledAt, LastCheckedAt: row.LastCheckedAt, NextCheckAt: row.NextCheckAt,
		LastHealthCheckedAt: row.LastHealthCheckedAt, LastHealthFailureAt: row.LastHealthFailureAt,
		ReconciliationAttempts: row.ReconciliationAttempts, ReconcileLockedAt: row.ReconcileLockedAt,
		ReconcileLockedBy: row.ReconcileLockedBy, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func domainFromSQLC(row dbsqlc.Domain, records []dbsqlc.DomainDnsRecord) SenderDomain {
	var createdBy *string
	if row.CreatedBy != nil {
		value := row.CreatedBy.String()
		createdBy = &value
	}
	return SenderDomain{
		ID:                        row.ID.String(),
		TeamID:                    row.TeamID.String(),
		Domain:                    row.NormalizedName,
		Provider:                  row.Provider,
		ProviderAccount:           row.ProviderAccount,
		ProviderRegion:            row.ProviderRegion,
		ProviderExternalID:        row.ProviderExternalID,
		Status:                    row.Status,
		ProviderStatus:            row.ProviderStatus,
		VerificationRecords:       verificationRecordsFromSQLC(records),
		OpenTracking:              row.OpenTracking,
		ClickTracking:             row.ClickTracking,
		TrackingSubdomain:         row.TrackingSubdomain,
		ActiveTrackingSubdomain:   row.ActiveTrackingSubdomain,
		TLS:                       row.TlsMode,
		Capabilities:              Capabilities{Sending: row.SendingEnabled, Receiving: row.ReceivingEnabled},
		CustomReturnPath:          row.CustomReturnPath,
		FailureReason:             row.FailureReason,
		HealthStatus:              row.HealthStatus,
		ConsecutiveHealthFailures: row.ConsecutiveHealthFailures,
		LastCheckedAt:             timestamptzPtr(row.LastCheckedAt),
		LastHealthCheckedAt:       timestamptzPtr(row.LastHealthCheckedAt),
		LastHealthFailureAt:       timestamptzPtr(row.LastHealthFailureAt),
		VerifiedAt:                timestamptzPtr(row.VerifiedAt),
		DisabledAt:                timestamptzPtr(row.DisabledAt),
		CreatedBy:                 createdBy,
		CreatedAt:                 row.CreatedAt.Time,
		UpdatedAt:                 row.UpdatedAt.Time,
	}
}

func verificationRecordsFromSQLC(rows []dbsqlc.DomainDnsRecord) []VerificationRecord {
	records := make([]VerificationRecord, 0, len(rows))
	for _, row := range rows {
		var priority *int
		if row.Priority != nil {
			value := int(*row.Priority)
			priority = &value
		}
		records = append(records, VerificationRecord{
			Record:   row.Record,
			Name:     row.Name,
			Value:    row.Value,
			Type:     row.Type,
			Status:   row.Status,
			TTL:      row.Ttl,
			Priority: priority,
		})
	}
	return records
}

func verificationRecordPurpose(record VerificationRecord) string {
	switch strings.ToUpper(strings.TrimSpace(record.Record)) {
	case platformemail.RecordDKIM:
		return "dkim"
	case platformemail.RecordSPF:
		if strings.EqualFold(strings.TrimSpace(record.Type), platformemail.RecordTypeMX) {
			return "mail_from"
		}
		return "spf"
	default:
		purpose := strings.ToLower(strings.TrimSpace(record.Record))
		if purpose == "" {
			return "spf"
		}
		return purpose
	}
}

func (r *Repository) requireConfigured() error {
	if r == nil || r.db == nil || r.queries == nil {
		return errors.New("sender domain repository is not configured")
	}
	return nil
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: !value.IsZero()}
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
