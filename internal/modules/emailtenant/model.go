package emailtenant

import (
	"time"

	"github.com/google/uuid"
)

const (
	ProviderAWSSES = "aws_ses"

	StatusPending      = "pending"
	StatusProvisioning = "provisioning"
	StatusActive       = "active"
	StatusPaused       = "paused"
	StatusDeleting     = "deleting"
	StatusFailed       = "failed"

	SuppressionScopeAccount = "account"
	SuppressionScopeTenant  = "tenant"

	ReputationPolicyNone     = "none"
	ReputationPolicyStandard = "standard"
	ReputationPolicyStrict   = "strict"
)

// Tenant is the provider-neutral record that binds a Dugble team to an email
// provider's regional tenant or reputation-isolation boundary.
type Tenant struct {
	ID               uuid.UUID `json:"id"`
	TeamID           uuid.UUID `json:"team_id"`
	Provider         string    `json:"provider"`
	Region           string    `json:"region"`
	ExternalName     string    `json:"external_name"`
	ExternalID       *string   `json:"external_id,omitempty"`
	TenantARN        *string   `json:"tenant_arn,omitempty"`
	Status           string    `json:"status"`
	SuppressionScope string    `json:"suppression_scope"`
	ReputationPolicy string    `json:"reputation_policy"`
	FailureReason    *string   `json:"failure_reason,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CreateParams contains server-owned values used to reserve an email tenant
// before asynchronous provider provisioning begins.
type CreateParams struct {
	TeamID           uuid.UUID
	Provider         string
	Region           string
	ExternalName     string
	SuppressionScope string
	ReputationPolicy string
}
