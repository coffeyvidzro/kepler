package emailtenant

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeTransaction struct {
	committed  bool
	rolledBack bool
}

func (transaction *fakeTransaction) Commit(context.Context) error {
	transaction.committed = true
	return nil
}

func (transaction *fakeTransaction) Rollback(context.Context) error {
	transaction.rolledBack = true
	return nil
}

type fakeTenantStore struct {
	transaction *fakeTransaction
	created     Tenant
	marked      Tenant
}

func (store *fakeTenantStore) BeginTx(context.Context) (Transaction, error) {
	if store.transaction == nil {
		store.transaction = &fakeTransaction{}
	}
	return store.transaction, nil
}

func (store *fakeTenantStore) CreateTx(context.Context, Transaction, CreateParams) (Tenant, error) {
	return store.created, nil
}

func (store *fakeTenantStore) MarkProvisioningTx(context.Context, Transaction, uuid.UUID) (Tenant, error) {
	return store.marked, nil
}

type fakeProvisionQueue struct {
	request ProvisioningRequest
	err     error
}

func (queue *fakeProvisionQueue) EnqueueProvisioningTx(_ context.Context, _ Transaction, request ProvisioningRequest) error {
	queue.request = request
	return queue.err
}

func TestRequestProvisioningCreatesRequestAtomically(t *testing.T) {
	teamID, tenantID := uuid.New(), uuid.New()
	transaction := &fakeTransaction{}
	store := &fakeTenantStore{
		transaction: transaction,
		created: Tenant{
			ID: tenantID, TeamID: teamID, Provider: ProviderAWSSES,
			Region: "eu-north-1", ExternalName: AWSExternalName(teamID), Status: StatusPending,
		},
		marked: Tenant{
			ID: tenantID, TeamID: teamID, Provider: ProviderAWSSES,
			Region: "eu-north-1", ExternalName: AWSExternalName(teamID), Status: StatusProvisioning,
			SuppressionScope: SuppressionScopeTenant, ReputationPolicy: ReputationPolicyStandard,
		},
	}
	queue := &fakeProvisionQueue{}

	got, err := NewService(store, queue).RequestProvisioning(context.Background(), teamID, " EU-NORTH-1 ")
	if err != nil {
		t.Fatalf("RequestProvisioning() error = %v", err)
	}
	if got.Status != StatusProvisioning || !transaction.committed {
		t.Fatalf("tenant = %#v, committed = %v", got, transaction.committed)
	}
	if queue.request.EventID == uuid.Nil || queue.request.TenantID != tenantID || queue.request.TeamID != teamID {
		t.Fatalf("unexpected provisioning request: %#v", queue.request)
	}
}

func TestRequestProvisioningRollsBackWhenQueueFails(t *testing.T) {
	teamID, tenantID := uuid.New(), uuid.New()
	transaction := &fakeTransaction{}
	store := &fakeTenantStore{
		transaction: transaction,
		created: Tenant{ID: tenantID, TeamID: teamID, Status: StatusPending},
		marked: Tenant{
			ID: tenantID, TeamID: teamID, Provider: ProviderAWSSES,
			Region: "eu-north-1", ExternalName: AWSExternalName(teamID), Status: StatusProvisioning,
		},
	}
	queueErr := errors.New("outbox unavailable")
	_, err := NewService(store, &fakeProvisionQueue{err: queueErr}).RequestProvisioning(
		context.Background(), teamID, "eu-north-1",
	)
	if !errors.Is(err, queueErr) || transaction.committed || !transaction.rolledBack {
		t.Fatalf("error = %v, committed = %v, rolled back = %v", err, transaction.committed, transaction.rolledBack)
	}
}
