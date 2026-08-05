package tenantprovision

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
)

type tenantStore interface {
	Get(context.Context, uuid.UUID) (emailtenant.Tenant, error)
	MarkActive(context.Context, uuid.UUID, string, string) (emailtenant.Tenant, error)
	MarkFailed(context.Context, uuid.UUID, error) (emailtenant.Tenant, error)
}

type Processor struct {
	store    tenantStore
	provider emailtenant.Provisioner
}

type Handler = Processor

func NewProcessor(store tenantStore, provider emailtenant.Provisioner) *Processor {
	return &Processor{store: store, provider: provider}
}

func NewHandler(store tenantStore, provider emailtenant.Provisioner) *Processor {
	return NewProcessor(store, provider)
}

func (processor *Processor) Handle(ctx context.Context, command Command) error {
	if processor == nil || processor.store == nil || processor.provider == nil {
		return ErrProcessorNotConfigured
	}
	if err := ValidateCommand(command); err != nil {
		return err
	}
	current, err := processor.store.Get(ctx, command.TenantID)
	if err != nil {
		return fmt.Errorf("load email tenant for provisioning: %w", err)
	}
	if current.TeamID != command.TeamID || current.Provider != command.Provider || current.Region != command.Region || current.ExternalName != command.ExternalName {
		return errors.New("email tenant provisioning command does not match persisted tenant")
	}
	if current.Status == emailtenant.StatusActive {
		return nil
	}
	if current.Status != emailtenant.StatusProvisioning {
		return fmt.Errorf("email tenant is %s, expected provisioning", current.Status)
	}

	result, err := processor.provider.ProvisionTenant(ctx, emailtenant.ProvisionRequest{
		Region:           current.Region,
		ExternalName:     current.ExternalName,
		SuppressionScope: current.SuppressionScope,
		ReputationPolicy: current.ReputationPolicy,
	})
	if err != nil {
		return fmt.Errorf("provision SES tenant: %w", err)
	}
	if _, err := processor.store.MarkActive(ctx, current.ID, result.ExternalID, result.TenantARN); err != nil {
		return fmt.Errorf("activate email tenant: %w", err)
	}
	return nil
}

func (processor *Processor) HandleExhausted(ctx context.Context, command Command, cause error) error {
	if processor == nil || processor.store == nil {
		return ErrProcessorNotConfigured
	}
	current, err := processor.store.Get(ctx, command.TenantID)
	if err != nil {
		return fmt.Errorf("load exhausted email tenant: %w", err)
	}
	if current.Status == emailtenant.StatusActive || current.Status == emailtenant.StatusFailed {
		return nil
	}
	if _, err := processor.store.MarkFailed(ctx, command.TenantID, cause); err != nil {
		return fmt.Errorf("mark email tenant provisioning failed: %w", err)
	}
	return nil
}
