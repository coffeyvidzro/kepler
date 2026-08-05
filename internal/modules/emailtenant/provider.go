package emailtenant

import "context"

// ProvisionRequest describes the provider-neutral desired state for one
// regional email tenant.
type ProvisionRequest struct {
	Region           string
	ExternalName     string
	SuppressionScope string
	ReputationPolicy string
}

// ProvisionResult identifies the tenant created or converged by a provider
// adapter.
type ProvisionResult struct {
	ExternalID string
	TenantARN  string
}

// Provisioner converges one provider tenant to the requested state.
type Provisioner interface {
	ProvisionTenant(context.Context, ProvisionRequest) (ProvisionResult, error)
}
