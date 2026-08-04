package tenantprovision

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	awsses "github.com/coffeyvidzro/dugble/server/internal/integration/aws/ses"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
)

type tenantStore interface {
	Get(context.Context, uuid.UUID) (emailtenant.Tenant, error)
	MarkActive(context.Context, uuid.UUID, string, string) (emailtenant.Tenant, error)
	MarkFailed(context.Context, uuid.UUID, error) (emailtenant.Tenant, error)
}

type tenantProvider interface {
	ProvisionTenant(context.Context, awsses.TenantProvisionRequest) (awsses.TenantProvisionResult, error)
}

type Handler struct {
	store    tenantStore
	provider tenantProvider
}

func NewHandler(store tenantStore, provider tenantProvider) *Handler {
	return &Handler{store: store, provider: provider}
}

func (h *Handler) Handle(ctx context.Context, command emailtenant.ProvisionCommand) error {
	if h == nil || h.store == nil || h.provider == nil {
		return errors.New("email tenant provisioning handler is not configured")
	}
	current, err := h.store.Get(ctx, command.TenantID)
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

	result, err := h.provider.ProvisionTenant(ctx, awsses.TenantProvisionRequest{
		Region:           current.Region,
		ExternalName:     current.ExternalName,
		SuppressionScope: current.SuppressionScope,
		ReputationPolicy: current.ReputationPolicy,
	})
	if err != nil {
		return fmt.Errorf("provision SES tenant: %w", err)
	}
	if _, err := h.store.MarkActive(ctx, current.ID, result.ExternalID, result.TenantARN); err != nil {
		return fmt.Errorf("activate email tenant: %w", err)
	}
	return nil
}

func (h *Handler) HandleExhausted(ctx context.Context, command emailtenant.ProvisionCommand, cause error) error {
	if h == nil || h.store == nil {
		return errors.New("email tenant provisioning handler store is not configured")
	}
	current, err := h.store.Get(ctx, command.TenantID)
	if err != nil {
		return fmt.Errorf("load exhausted email tenant: %w", err)
	}
	if current.Status == emailtenant.StatusActive || current.Status == emailtenant.StatusFailed {
		return nil
	}
	if _, err := h.store.MarkFailed(ctx, command.TenantID, cause); err != nil {
		return fmt.Errorf("mark email tenant provisioning failed: %w", err)
	}
	return nil
}
