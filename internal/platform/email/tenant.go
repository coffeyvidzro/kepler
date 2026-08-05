package email

import "context"

// TenantProvisionRequest describes the provider-neutral desired state for one
// regional email tenant.
type TenantProvisionRequest struct {
	Region           string
	ExternalName     string
	SuppressionScope string
	ReputationPolicy string
}

// TenantProvisionResult identifies the tenant created or converged by a
// provider adapter.
type TenantProvisionResult struct {
	ExternalID string
	TenantARN  string
}

// TenantProvisioner converges one provider tenant to the requested state.
type TenantProvisioner interface {
	ProvisionTenant(context.Context, TenantProvisionRequest) (TenantProvisionResult, error)
}
