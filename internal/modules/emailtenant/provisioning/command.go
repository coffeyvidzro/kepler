package tenantprovision

import (
	"errors"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
)

const (
	ProvisionSubject   = "dugble.job.email.tenant.provision.v1"
	ProvisionEventType = "email.tenant.provision.requested.v1"
	ConsumerName       = "dugble-email-tenant-provision-v1"
	DLQSubject         = "dugble.dlq.email.tenant.provision.v1"
)

type Command struct {
	EventID          uuid.UUID `json:"event_id"`
	TenantID         uuid.UUID `json:"tenant_id"`
	TeamID           uuid.UUID `json:"team_id"`
	Provider         string    `json:"provider"`
	Region           string    `json:"region"`
	ExternalName     string    `json:"external_name"`
	SuppressionScope string    `json:"suppression_scope"`
	ReputationPolicy string    `json:"reputation_policy"`
	SchemaVersion    int       `json:"schema_version"`
}

func commandFromRequest(request emailtenant.ProvisioningRequest) Command {
	return Command{
		EventID:          request.EventID,
		TenantID:         request.TenantID,
		TeamID:           request.TeamID,
		Provider:         request.Provider,
		Region:           request.Region,
		ExternalName:     request.ExternalName,
		SuppressionScope: request.SuppressionScope,
		ReputationPolicy: request.ReputationPolicy,
		SchemaVersion:    1,
	}
}

func ValidateCommand(command Command) error {
	if command.EventID == uuid.Nil || command.TenantID == uuid.Nil || command.TeamID == uuid.Nil {
		return errors.New("email tenant provisioning command identifiers are required")
	}
	if command.SchemaVersion != 1 {
		return errors.New("unsupported email tenant provisioning schema version")
	}
	return nil
}
