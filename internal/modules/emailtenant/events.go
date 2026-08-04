package emailtenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
)

const (
	ProvisionSubject   = "dugble.job.email.tenant.provision.v1"
	ProvisionEventType = "email.tenant.provision.requested.v1"
)

type ProvisionCommand struct {
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

type outboxStore interface {
	EnqueueTx(context.Context, pgx.Tx, outbox.Event) (uuid.UUID, error)
}

// ProvisionQueue writes tenant provisioning commands to the PostgreSQL outbox.
type ProvisionQueue struct {
	store outboxStore
}

func NewProvisionQueue(store outboxStore) *ProvisionQueue {
	return &ProvisionQueue{store: store}
}

func (q *ProvisionQueue) EnqueueProvisioningTx(ctx context.Context, tx Transaction, command ProvisionCommand) error {
	if q == nil || q.store == nil {
		return errors.New("email tenant provisioning queue is not configured")
	}
	pgxTx, err := requirePGXTx(tx)
	if err != nil {
		return err
	}
	if command.EventID == uuid.Nil || command.TenantID == uuid.Nil || command.TeamID == uuid.Nil {
		return errors.New("email tenant provisioning command identifiers are required")
	}
	if command.SchemaVersion == 0 {
		command.SchemaVersion = 1
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("encode email tenant provisioning command: %w", err)
	}
	_, err = q.store.EnqueueTx(ctx, pgxTx, outbox.Event{
		ID:            command.EventID,
		Subject:       ProvisionSubject,
		AggregateType: "email_tenant",
		AggregateID:   command.TenantID,
		Payload:       payload,
		Headers: map[string]string{
			"Dugble-Event-Type": ProvisionEventType,
		},
	})
	if err != nil {
		return fmt.Errorf("enqueue email tenant provisioning command: %w", err)
	}
	return nil
}
