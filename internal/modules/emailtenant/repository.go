package emailtenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
)

var (
	ErrNotFound          = errors.New("email tenant not found")
	ErrInvalidTransition = errors.New("email tenant state transition is not allowed")
)

// Transaction is the subset of pgx.Tx needed by the service and its tests.
type Transaction interface {
	Commit(context.Context) error
	Rollback(context.Context) error
}

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func (r *Repository) BeginTx(ctx context.Context) (Transaction, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("email tenant repository is not configured")
	}
	return r.db.BeginTx(ctx, pgx.TxOptions{})
}

func (r *Repository) CreateTx(ctx context.Context, tx Transaction, params CreateParams) (Tenant, error) {
	pgxTx, err := requirePGXTx(tx)
	if err != nil {
		return Tenant{}, err
	}
	row, err := r.queries.WithTx(pgxTx).CreateEmailTenant(ctx, dbsqlc.CreateEmailTenantParams{
		TeamID:           params.TeamID,
		Provider:         strings.ToLower(strings.TrimSpace(params.Provider)),
		Region:           strings.ToLower(strings.TrimSpace(params.Region)),
		ExternalName:     strings.ToLower(strings.TrimSpace(params.ExternalName)),
		SuppressionScope: strings.ToLower(strings.TrimSpace(params.SuppressionScope)),
		ReputationPolicy: strings.ToLower(strings.TrimSpace(params.ReputationPolicy)),
	})
	if err != nil {
		return Tenant{}, fmt.Errorf("create email tenant: %w", err)
	}
	return tenantFromSQLC(row), nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Tenant, error) {
	row, err := r.queries.GetEmailTenant(ctx, dbsqlc.GetEmailTenantParams{ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("get email tenant: %w", err)
	}
	return tenantFromSQLC(row), nil
}

func (r *Repository) GetByTeam(ctx context.Context, teamID uuid.UUID, provider, region string) (Tenant, error) {
	row, err := r.queries.GetEmailTenantByTeamProviderRegion(ctx, dbsqlc.GetEmailTenantByTeamProviderRegionParams{
		TeamID:   teamID,
		Provider: strings.ToLower(strings.TrimSpace(provider)),
		Region:   strings.ToLower(strings.TrimSpace(region)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("get email tenant by team: %w", err)
	}
	return tenantFromSQLC(row), nil
}

func (r *Repository) MarkProvisioningTx(ctx context.Context, tx Transaction, id uuid.UUID) (Tenant, error) {
	pgxTx, err := requirePGXTx(tx)
	if err != nil {
		return Tenant{}, err
	}
	row, err := r.queries.WithTx(pgxTx).MarkEmailTenantProvisioning(ctx, dbsqlc.MarkEmailTenantProvisioningParams{ID: id})
	return lifecycleResult(row, err, "mark email tenant provisioning")
}

func (r *Repository) MarkActive(ctx context.Context, id uuid.UUID, externalID, tenantARN string) (Tenant, error) {
	externalID = strings.TrimSpace(externalID)
	tenantARN = strings.TrimSpace(tenantARN)
	if externalID == "" {
		return Tenant{}, errors.New("email tenant external id is required")
	}
	if tenantARN == "" {
		return Tenant{}, errors.New("email tenant ARN is required")
	}
	row, err := r.queries.MarkEmailTenantActive(ctx, dbsqlc.MarkEmailTenantActiveParams{ID: id, ExternalID: &externalID, TenantArn: &tenantARN})
	return lifecycleResult(row, err, "mark email tenant active")
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, cause error) (Tenant, error) {
	reason := "email tenant provisioning failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		reason = strings.TrimSpace(cause.Error())
	}
	row, err := r.queries.MarkEmailTenantFailed(ctx, dbsqlc.MarkEmailTenantFailedParams{ID: id, FailureReason: &reason})
	return lifecycleResult(row, err, "mark email tenant failed")
}

func (r *Repository) MarkPaused(ctx context.Context, id uuid.UUID, reason string) (Tenant, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "email tenant paused"
	}
	row, err := r.queries.MarkEmailTenantPaused(ctx, dbsqlc.MarkEmailTenantPausedParams{ID: id, FailureReason: &reason})
	return lifecycleResult(row, err, "mark email tenant paused")
}

func (r *Repository) MarkDeleting(ctx context.Context, id uuid.UUID) (Tenant, error) {
	row, err := r.queries.MarkEmailTenantDeleting(ctx, dbsqlc.MarkEmailTenantDeletingParams{ID: id})
	return lifecycleResult(row, err, "mark email tenant deleting")
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := r.queries.DeleteEmailTenant(ctx, dbsqlc.DeleteEmailTenantParams{ID: id})
	if err != nil {
		return fmt.Errorf("delete email tenant: %w", err)
	}
	if rows == 0 {
		return ErrInvalidTransition
	}
	return nil
}

func requirePGXTx(tx Transaction) (pgx.Tx, error) {
	pgxTx, ok := tx.(pgx.Tx)
	if !ok || pgxTx == nil {
		return nil, errors.New("email tenant transaction is invalid")
	}
	return pgxTx, nil
}

func lifecycleResult(row dbsqlc.EmailTenant, err error, operation string) (Tenant, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrInvalidTransition
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("%s: %w", operation, err)
	}
	return tenantFromSQLC(row), nil
}

func tenantFromSQLC(row dbsqlc.EmailTenant) Tenant {
	return Tenant{
		ID:               row.ID,
		TeamID:           row.TeamID,
		Provider:         row.Provider,
		Region:           row.Region,
		ExternalName:     row.ExternalName,
		ExternalID:       row.ExternalID,
		TenantARN:        row.TenantArn,
		Status:           row.Status,
		SuppressionScope: row.SuppressionScope,
		ReputationPolicy: row.ReputationPolicy,
		FailureReason:    row.FailureReason,
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}
