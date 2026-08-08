package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

var (
	ErrDomainClaimAlreadyExists = errors.New("domain claim already exists")
	ErrDomainAlreadyOwned       = errors.New("domain is already owned by the team")
)

type DomainClaimReconciliation struct {
	Claim DomainClaim
}

func (r *Repository) CreateClaim(
	ctx context.Context,
	targetTeamID uuid.UUID,
	createdBy uuid.UUID,
	name, region string,
	configuration DomainConfiguration,
) (DomainClaim, error) {
	if err := r.requireConfigured(); err != nil {
		return DomainClaim{}, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DomainClaim{}, fmt.Errorf("begin domain claim creation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	source, err := queries.GetDomainByNameForClaim(ctx, dbsqlc.GetDomainByNameForClaimParams{Name: name})
	if err != nil {
		return DomainClaim{}, err
	}
	if source.TeamID == targetTeamID {
		return DomainClaim{}, ErrDomainAlreadyOwned
	}

	targetDomainID := uuid.New()
	verificationToken := uuid.NewString()
	row, err := queries.CreateDomainClaim(ctx, dbsqlc.CreateDomainClaimParams{
		TargetDomainID:    targetDomainID,
		SourceDomainID:    &source.ID,
		NormalizedName:    source.NormalizedName,
		SourceTeamID:      source.TeamID,
		TargetTeamID:      targetTeamID,
		ProviderRegion:    region,
		CustomReturnPath:  configuration.CustomReturnPath,
		OpenTracking:      configuration.OpenTracking,
		ClickTracking:     configuration.ClickTracking,
		TrackingSubdomain: configuration.TrackingSubdomain,
		TlsMode:           configuration.TLS,
		SendingEnabled:    configuration.Capabilities.Sending,
		ReceivingEnabled:  configuration.Capabilities.Receiving,
		RecordName:        "_kepler-claim." + source.NormalizedName,
		RecordValue:       verificationToken,
		RecordTtl:         "Auto",
		ExpiresAt:         timestamptz(time.Now().UTC().Add(DefaultClaimLifetime)),
		CreatedBy:         &createdBy,
	})
	if isUniqueViolation(err) {
		return DomainClaim{}, ErrDomainClaimAlreadyExists
	}
	if err != nil {
		return DomainClaim{}, fmt.Errorf("create domain claim: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DomainClaim{}, fmt.Errorf("commit domain claim creation: %w", err)
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) GetClaim(
	ctx context.Context,
	targetDomainID, targetTeamID uuid.UUID,
) (DomainClaim, error) {
	row, err := r.queries.GetDomainClaim(ctx, dbsqlc.GetDomainClaimParams{
		TargetDomainID: targetDomainID,
		TargetTeamID:   targetTeamID,
	})
	if err != nil {
		return DomainClaim{}, err
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) RequestClaimVerification(
	ctx context.Context,
	targetDomainID, targetTeamID uuid.UUID,
) (DomainClaim, error) {
	row, err := r.queries.RequestDomainClaimVerification(ctx, dbsqlc.RequestDomainClaimVerificationParams{
		TargetDomainID: targetDomainID,
		TargetTeamID:   targetTeamID,
	})
	if err != nil {
		return DomainClaim{}, err
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) CancelClaim(
	ctx context.Context,
	targetDomainID, targetTeamID uuid.UUID,
) (DomainClaim, error) {
	row, err := r.queries.CancelDomainClaim(ctx, dbsqlc.CancelDomainClaimParams{
		TargetDomainID: targetDomainID,
		TargetTeamID:   targetTeamID,
	})
	if err != nil {
		return DomainClaim{}, err
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) ClaimPendingClaims(
	ctx context.Context,
	workerID string,
	limit int32,
	staleBefore time.Time,
) ([]DomainClaimReconciliation, error) {
	rows, err := r.queries.ClaimPendingDomainClaims(ctx, dbsqlc.ClaimPendingDomainClaimsParams{
		StaleBefore: timestamptz(staleBefore),
		ClaimLimit:  limit,
		WorkerID:    strings.TrimSpace(workerID),
	})
	if err != nil {
		return nil, fmt.Errorf("claim pending domain claims: %w", err)
	}
	claims := make([]DomainClaimReconciliation, 0, len(rows))
	for _, row := range rows {
		claims = append(claims, DomainClaimReconciliation{Claim: domainClaimFromPendingRow(row)})
	}
	return claims, nil
}

func (r *Repository) ReleaseClaim(
	ctx context.Context,
	claimID uuid.UUID,
	workerID string,
) (DomainClaim, error) {
	row, err := r.queries.ReleaseDomainClaim(ctx, dbsqlc.ReleaseDomainClaimParams{
		ID: claimID, WorkerID: strings.TrimSpace(workerID),
	})
	if err != nil {
		return DomainClaim{}, err
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) MarkClaimVerified(
	ctx context.Context,
	claimID uuid.UUID,
	workerID string,
) (DomainClaim, error) {
	row, err := r.queries.MarkDomainClaimVerified(ctx, dbsqlc.MarkDomainClaimVerifiedParams{
		ID: claimID, WorkerID: strings.TrimSpace(workerID),
	})
	if err != nil {
		return DomainClaim{}, err
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) MarkClaimBlocked(
	ctx context.Context,
	claimID uuid.UUID,
	workerID, reason string,
) (DomainClaim, error) {
	row, err := r.queries.MarkDomainClaimBlocked(ctx, dbsqlc.MarkDomainClaimBlockedParams{
		BlockedReason: &reason,
		ID:            claimID,
		WorkerID:      strings.TrimSpace(workerID),
	})
	if err != nil {
		return DomainClaim{}, err
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) MarkClaimFailed(
	ctx context.Context,
	claimID uuid.UUID,
	workerID string,
	cause error,
) (DomainClaim, error) {
	reason := "domain claim reconciliation failed"
	if cause != nil {
		reason = cause.Error()
	}
	row, err := r.queries.MarkDomainClaimFailed(ctx, dbsqlc.MarkDomainClaimFailedParams{
		FailureReason: &reason,
		ID:            claimID,
		WorkerID:      strings.TrimSpace(workerID),
	})
	if err != nil {
		return DomainClaim{}, err
	}
	return domainClaimFromSQLC(row), nil
}

func (r *Repository) HasPendingScheduledEmails(ctx context.Context, domainID uuid.UUID) (bool, error) {
	return r.queries.DomainHasPendingScheduledEmails(ctx, dbsqlc.DomainHasPendingScheduledEmailsParams{DomainID: &domainID})
}

func (r *Repository) CompleteClaimTransfer(
	ctx context.Context,
	claimID uuid.UUID,
	workerID string,
	records []VerificationRecord,
) (SenderDomain, DomainClaim, error) {
	if err := r.requireConfigured(); err != nil {
		return SenderDomain{}, DomainClaim{}, err
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return SenderDomain{}, DomainClaim{}, fmt.Errorf("begin domain claim completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	queries := r.queries.WithTx(tx)
	claim, err := queries.GetDomainClaimByID(ctx, dbsqlc.GetDomainClaimByIDParams{ID: claimID})
	if err != nil {
		return SenderDomain{}, DomainClaim{}, fmt.Errorf("get domain claim for completion: %w", err)
	}
	if claim.Status != ClaimStatusVerified || claim.SourceDomainID == nil {
		return SenderDomain{}, DomainClaim{}, errors.New("domain claim is not ready for completion")
	}
	source, err := queries.DeleteDomainForClaim(ctx, dbsqlc.DeleteDomainForClaimParams{ID: *claim.SourceDomainID})
	if err != nil {
		return SenderDomain{}, DomainClaim{}, fmt.Errorf("remove source domain during claim: %w", err)
	}
	created, err := queries.CreateClaimedDomain(ctx, dbsqlc.CreateClaimedDomainParams{
		ID:                claim.TargetDomainID,
		TeamID:            claim.TargetTeamID,
		Name:              claim.NormalizedName,
		Provider:          source.Provider,
		ProviderAccount:   source.ProviderAccount,
		ProviderRegion:    claim.ProviderRegion,
		OpenTracking:      claim.OpenTracking,
		ClickTracking:     claim.ClickTracking,
		TrackingSubdomain: claim.TrackingSubdomain,
		TlsMode:           claim.TlsMode,
		SendingEnabled:    claim.SendingEnabled,
		ReceivingEnabled:  claim.ReceivingEnabled,
		CustomReturnPath:  claim.CustomReturnPath,
		CreatedBy:         claim.CreatedBy,
	})
	if err != nil {
		return SenderDomain{}, DomainClaim{}, fmt.Errorf("create claimed domain: %w", err)
	}
	if err := replaceDomainDNSRecords(ctx, queries, created.ID, records); err != nil {
		return SenderDomain{}, DomainClaim{}, err
	}
	completed, err := queries.MarkDomainClaimCompleted(ctx, dbsqlc.MarkDomainClaimCompletedParams{
		ID:       claim.ID,
		WorkerID: strings.TrimSpace(workerID),
	})
	if err != nil {
		return SenderDomain{}, DomainClaim{}, fmt.Errorf("complete domain claim: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SenderDomain{}, DomainClaim{}, fmt.Errorf("commit domain claim completion: %w", err)
	}
	return domainFromSQLC(created, nil), domainClaimFromSQLC(completed), nil
}

func (s *Service) StartClaim(ctx context.Context, req ClaimRequest) (DomainClaim, error) {
	tc, err := requireTenantPermission(ctx, tenant.PermissionSenderDomainsCreate)
	if err != nil {
		return DomainClaim{}, err
	}
	name, region, configuration, err := validateClaim(req)
	if err != nil {
		return DomainClaim{}, err
	}
	claim, err := s.repository.CreateClaim(
		ctx,
		tc.Scope.TeamID,
		tc.Actor.UserID,
		name,
		region,
		configuration,
	)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return DomainClaim{}, apperrors.NewNotFound("Sender domain is not currently owned by another team")
	case errors.Is(err, ErrDomainAlreadyOwned):
		return DomainClaim{}, apperrors.NewConflict("Sender domain is already owned by this team")
	case errors.Is(err, ErrDomainClaimAlreadyExists):
		return DomainClaim{}, apperrors.NewConflict("An active claim already exists for this sender domain")
	case err != nil:
		return DomainClaim{}, apperrors.NewInternal("Unable to create sender domain claim", err)
	default:
		return claim, nil
	}
}

func (s *Service) GetClaim(ctx context.Context, domainID string) (DomainClaim, error) {
	tc, err := requireTenantPermission(ctx, tenant.PermissionSenderDomainsRead)
	if err != nil {
		return DomainClaim{}, err
	}
	id, err := parseDomainID(domainID)
	if err != nil {
		return DomainClaim{}, err
	}
	claim, err := s.repository.GetClaim(ctx, id, tc.Scope.TeamID)
	if err != nil {
		return DomainClaim{}, apperrors.NewNotFound("Sender domain claim not found")
	}
	return claim, nil
}

func (s *Service) VerifyClaim(ctx context.Context, domainID string) (DomainClaim, error) {
	tc, err := requireTenantPermission(ctx, tenant.PermissionSenderDomainsCreate)
	if err != nil {
		return DomainClaim{}, err
	}
	id, err := parseDomainID(domainID)
	if err != nil {
		return DomainClaim{}, err
	}
	claim, err := s.repository.RequestClaimVerification(ctx, id, tc.Scope.TeamID)
	if err != nil {
		return DomainClaim{}, apperrors.NewNotFound("Active sender domain claim not found")
	}
	return claim, nil
}

func (s *Service) CancelClaim(ctx context.Context, domainID string) (DomainClaim, error) {
	tc, err := requireTenantPermission(ctx, tenant.PermissionSenderDomainsDelete)
	if err != nil {
		return DomainClaim{}, err
	}
	id, err := parseDomainID(domainID)
	if err != nil {
		return DomainClaim{}, err
	}
	claim, err := s.repository.CancelClaim(ctx, id, tc.Scope.TeamID)
	if err != nil {
		return DomainClaim{}, apperrors.NewNotFound("Active sender domain claim not found")
	}
	return claim, nil
}

// ReconcileClaim performs one idempotent worker iteration for an ownership
// claim that has been locked by ClaimPendingClaims.
func (s *Service) ReconcileClaim(
	ctx context.Context,
	claim DomainClaim,
	workerID string,
) (DomainClaim, error) {
	claimID, err := uuid.Parse(claim.ID)
	if err != nil {
		return DomainClaim{}, fmt.Errorf("parse domain claim ID: %w", err)
	}
	if s.dns == nil || s.provider == nil || s.tenantProvision == nil {
		return DomainClaim{}, errors.New("domain claim reconciliation is not configured")
	}
	if !s.dns.Verify(ctx, claim.Name, claim.VerificationRecord) {
		return s.repository.ReleaseClaim(ctx, claimID, workerID)
	}
	if claim.SourceDomainID == nil {
		return s.repository.MarkClaimFailed(
			ctx,
			claimID,
			workerID,
			errors.New("claimed source domain no longer exists"),
		)
	}
	sourceDomainID, err := uuid.Parse(*claim.SourceDomainID)
	if err != nil {
		return DomainClaim{}, fmt.Errorf("parse claimed source domain ID: %w", err)
	}
	pending, err := s.repository.HasPendingScheduledEmails(ctx, sourceDomainID)
	if err != nil {
		return DomainClaim{}, fmt.Errorf("check pending claimed-domain messages: %w", err)
	}
	if pending {
		return s.repository.MarkClaimBlocked(
			ctx,
			claimID,
			workerID,
			ClaimBlockedPendingScheduledEmails,
		)
	}
	verified, err := s.repository.MarkClaimVerified(ctx, claimID, workerID)
	if err != nil {
		return DomainClaim{}, fmt.Errorf("mark domain claim verified: %w", err)
	}
	source, err := s.repository.getByID(ctx, sourceDomainID)
	if err != nil {
		return DomainClaim{}, fmt.Errorf("load claimed source domain: %w", err)
	}
	targetTeamID, err := uuid.Parse(verified.TargetTeamID)
	if err != nil {
		return DomainClaim{}, fmt.Errorf("parse claim target team ID: %w", err)
	}
	emailTenant, err := s.tenantProvision.RequestProvisioning(ctx, targetTeamID, verified.Region)
	if err != nil {
		return DomainClaim{}, fmt.Errorf("prepare claimed domain email tenant: %w", err)
	}
	if emailTenant.Status != emailtenant.StatusActive {
		return s.repository.ReleaseClaim(ctx, claimID, workerID)
	}
	if err := s.provider.DeleteDomain(ctx, source.Domain, source.ProviderRegion); err != nil {
		return DomainClaim{}, fmt.Errorf("remove claimed domain from source provider route: %w", err)
	}
	records, err := s.provider.ProvisionDomain(ctx, platformemail.DomainProvisionRequest{
		Domain:           verified.Name,
		Region:           verified.Region,
		CustomReturnPath: verified.CustomReturnPath,
		SESTenantName:    emailTenant.ExternalName,
	})
	if err != nil {
		return DomainClaim{}, fmt.Errorf("provision claimed domain for target team: %w", err)
	}
	_, completed, err := s.repository.CompleteClaimTransfer(ctx, claimID, workerID, records)
	if err != nil {
		return DomainClaim{}, err
	}
	return completed, nil
}

func domainClaimFromPendingRow(row dbsqlc.ClaimPendingDomainClaimsRow) DomainClaim {
	return domainClaimFromSQLC(dbsqlc.DomainClaim(row))
}

func domainClaimFromSQLC(row dbsqlc.DomainClaim) DomainClaim {
	var sourceDomainID *string
	if row.SourceDomainID != nil {
		value := row.SourceDomainID.String()
		sourceDomainID = &value
	}
	return DomainClaim{
		ID:                row.ID.String(),
		DomainID:          row.TargetDomainID.String(),
		Name:              row.NormalizedName,
		Status:            row.Status,
		SourceDomainID:    sourceDomainID,
		SourceTeamID:      row.SourceTeamID.String(),
		TargetTeamID:      row.TargetTeamID.String(),
		Region:            row.ProviderRegion,
		CustomReturnPath:  row.CustomReturnPath,
		OpenTracking:      row.OpenTracking,
		ClickTracking:     row.ClickTracking,
		TrackingSubdomain: row.TrackingSubdomain,
		TLS:               row.TlsMode,
		Capabilities: Capabilities{
			Sending:   row.SendingEnabled,
			Receiving: row.ReceivingEnabled,
		},
		VerificationRecord: VerificationRecord{
			Record: "claim",
			Name:   row.RecordName,
			Value:  row.RecordValue,
			Type:   platformemail.RecordTypeTXT,
			Status: claimRecordStatus(row.Status),
			TTL:    row.RecordTtl,
		},
		BlockedReason:           row.BlockedReason,
		FailureReason:           row.FailureReason,
		VerificationRequestedAt: timestamptzPtr(row.VerificationRequestedAt),
		VerifiedAt:              timestamptzPtr(row.VerifiedAt),
		CompletedAt:             timestamptzPtr(row.CompletedAt),
		ExpiresAt:               row.ExpiresAt.Time,
		CreatedAt:               row.CreatedAt.Time,
		UpdatedAt:               row.UpdatedAt.Time,
	}
}

func claimRecordStatus(status string) string {
	if status == ClaimStatusVerified || status == ClaimStatusCompleted {
		return platformemail.RecordStatusVerified
	}
	if status == ClaimStatusFailed || status == ClaimStatusExpired || status == ClaimStatusCanceled {
		return platformemail.RecordStatusFailed
	}
	return platformemail.RecordStatusPending
}
