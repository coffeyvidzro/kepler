package emailtenant

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type tenantStore interface {
	BeginTx(context.Context) (Transaction, error)
	CreateTx(context.Context, Transaction, CreateParams) (Tenant, error)
	MarkProvisioningTx(context.Context, Transaction, uuid.UUID) (Tenant, error)
}

type provisioningQueue interface {
	EnqueueProvisioningTx(context.Context, Transaction, ProvisionCommand) error
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
func (s *Service) RequestProvisioning(ctx context.Context, teamID uuid.UUID, region string) (Tenant, error) {
	if s == nil || s.repository == nil {
		return Tenant{}, errors.New("email tenant service is not configured")
	}
	if s.queue == nil {
		return Tenant{}, errors.New("email tenant provisioning queue is not configured")
	}
	if teamID == uuid.Nil {
		return Tenant{}, errors.New("email tenant team id is required")
	}
	region, supported := platformemail.NormalizeSESRegion(region)
	if !supported {
		return Tenant{}, fmt.Errorf("unsupported SES region %q", region)
	}

	tx, err := s.repository.BeginTx(ctx)
	if err != nil {
		return Tenant{}, fmt.Errorf("begin email tenant provisioning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tenant, err := s.repository.CreateTx(ctx, tx, CreateParams{
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
		// Continue below and claim the lifecycle transition in this transaction.
	default:
		return Tenant{}, fmt.Errorf("unsupported email tenant status %q", tenant.Status)
	}

	tenant, err = s.repository.MarkProvisioningTx(ctx, tx, tenant.ID)
	if err != nil {
		return Tenant{}, err
	}
	command := ProvisionCommand{
		EventID:          uuid.New(),
		TenantID:         tenant.ID,
		TeamID:           tenant.TeamID,
		Provider:         tenant.Provider,
		Region:           tenant.Region,
		ExternalName:     tenant.ExternalName,
		SuppressionScope: tenant.SuppressionScope,
		ReputationPolicy: tenant.ReputationPolicy,
		SchemaVersion:    1,
	}
	if err := s.queue.EnqueueProvisioningTx(ctx, tx, command); err != nil {
		return Tenant{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Tenant{}, fmt.Errorf("commit email tenant provisioning transaction: %w", err)
	}
	return tenant, nil
}
