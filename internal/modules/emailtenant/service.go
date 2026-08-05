package emailtenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
)

type ProvisioningRequest struct {
	EventID          uuid.UUID
	TenantID         uuid.UUID
	TeamID           uuid.UUID
	Provider         string
	Region           string
	ExternalName     string
	SuppressionScope string
	ReputationPolicy string
}

type tenantStore interface {
	BeginTx(context.Context) (Transaction, error)
	CreateTx(context.Context, Transaction, CreateParams) (Tenant, error)
	MarkProvisioningTx(context.Context, Transaction, uuid.UUID) (Tenant, error)
}

type provisioningQueue interface {
	EnqueueProvisioningTx(context.Context, Transaction, ProvisioningRequest) error
}

type Service struct {
	repository tenantStore
	queue      provisioningQueue
}

func NewService(repository tenantStore, queue provisioningQueue) *Service {
	return &Service{repository: repository, queue: queue}
}

// RequestProvisioning reserves one regional provider tenant for a team and
// atomically publishes a provisioning command through the PostgreSQL outbox.
func (service *Service) RequestProvisioning(ctx context.Context, teamID uuid.UUID, region string) (Tenant, error) {
	if service == nil || service.repository == nil {
		return Tenant{}, errors.New("email tenant service is not configured")
	}
	if service.queue == nil {
		return Tenant{}, errors.New("email tenant provisioning queue is not configured")
	}
	if teamID == uuid.Nil {
		return Tenant{}, errors.New("email tenant team id is required")
	}
	region, supported := platformemail.NormalizeSESRegion(region)
	if !supported {
		return Tenant{}, fmt.Errorf("unsupported SES region %q", region)
	}

	tx, err := service.repository.BeginTx(ctx)
	if err != nil {
		return Tenant{}, fmt.Errorf("begin email tenant provisioning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tenant, err := service.repository.CreateTx(ctx, tx, CreateParams{
		TeamID:           teamID,
		Provider:         ProviderAWSSES,
		Region:           region,
		ExternalName:     AWSExternalName(teamID),
		SuppressionScope: SuppressionScopeTenant,
		ReputationPolicy: ReputationPolicyStandard,
	})
	if err != nil {
		return Tenant{}, err
	}

	switch tenant.Status {
	case StatusProvisioning, StatusActive, StatusPaused, StatusDeleting:
		if err := tx.Commit(ctx); err != nil {
			return Tenant{}, fmt.Errorf("commit existing email tenant transaction: %w", err)
		}
		return tenant, nil
	case StatusPending, StatusFailed:
	default:
		return Tenant{}, fmt.Errorf("unsupported email tenant status %q", tenant.Status)
	}

	tenant, err = service.repository.MarkProvisioningTx(ctx, tx, tenant.ID)
	if err != nil {
		return Tenant{}, err
	}
	request := ProvisioningRequest{
		EventID:          uuid.New(),
		TenantID:         tenant.ID,
		TeamID:           tenant.TeamID,
		Provider:         tenant.Provider,
		Region:           tenant.Region,
		ExternalName:     tenant.ExternalName,
		SuppressionScope: tenant.SuppressionScope,
		ReputationPolicy: tenant.ReputationPolicy,
	}
	if err := service.queue.EnqueueProvisioningTx(ctx, tx, request); err != nil {
		return Tenant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Tenant{}, fmt.Errorf("commit email tenant provisioning transaction: %w", err)
	}
	return tenant, nil
}
