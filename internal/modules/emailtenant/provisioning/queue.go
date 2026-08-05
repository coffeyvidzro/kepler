package tenantprovision

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	"github.com/coffeyvidzro/dugble/server/internal/platform/outbox"
)

type outboxStore interface {
	EnqueueTx(context.Context, pgx.Tx, outbox.Event) (uuid.UUID, error)
}

type Queue struct {
	store outboxStore
}

func NewQueue(store outboxStore) *Queue {
	return &Queue{store: store}
}

func (queue *Queue) EnqueueProvisioningTx(ctx context.Context, tx emailtenant.Transaction, request emailtenant.ProvisioningRequest) error {
	if queue == nil || queue.store == nil {
		return ErrQueueNotConfigured
	}
	pgxTx, ok := tx.(pgx.Tx)
	if !ok || pgxTx == nil {
		return errorsNewTransactionRequired()
	}
	command := commandFromRequest(request)
	if err := ValidateCommand(command); err != nil {
		return err
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("encode email tenant provisioning command: %w", err)
	}
	_, err = queue.store.EnqueueTx(ctx, pgxTx, outbox.Event{
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
