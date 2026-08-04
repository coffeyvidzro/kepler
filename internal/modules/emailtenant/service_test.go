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

func (tx *fakeTransaction) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *fakeTransaction) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

type fakeTenantStore struct {
	tx           *fakeTransaction
	created      Tenant
	createParams CreateParams
	markCalls    int
	markResult   Tenant
	beginErr     error
	createErr    error
	markErr      error
}

func (s *fakeTenantStore) BeginTx(context.Context) (Transaction, error) {
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	if s.tx == nil {
		s.tx = &fakeTransaction{}
	}
	return s.tx, nil
}

func (s *fakeTenantStore) CreateTx(_ context.Context, _ Transaction, params CreateParams) (Tenant, error) {
	s.createParams = params
	return s.created, s.createErr
}

func (s *fakeTenantStore) MarkProvisioningTx(context.Context, Transaction, uuid.UUID) (Tenant, error) {
	s.markCalls++
	return s.markResult, s.markErr
}

type fakeProvisionQueue struct {
	calls   int
	command ProvisionCommand
	err     error
}

func (q *fakeProvisionQueue) EnqueueProvisioningTx(_ context.Context, _ Transaction, command ProvisionCommand) error {
	q.calls++
	q.command = command
	return q.err
}

func TestRequestProvisioningCreatesCommandAtomically(t *testing.T) {
	teamID := uuid.MustParse("80d3f812-8ae4-4e19-aef4-16d93fa64015")
	tenantID := uuid.New()
	tx := &fakeTransaction{}
	store := &fakeTenantStore{
		tx: tx,
		created: Tenant{
			ID: tenantID, TeamID: teamID, Provider: ProviderAWSSES,
			Region: "eu-north-1", ExternalName: AWSExternalName(teamID), Status: StatusPending,
			SuppressionScope: SuppressionScopeTenant, ReputationPolicy: ReputationPolicyStandard,
		},
		markResult: Tenant{
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
	if got.Status != StatusProvisioning {
		t.Fatalf("status = %q, want %q", got.Status, StatusProvisioning)
	}
	if !tx.committed {
		t.Fatal("transaction was not committed")
	}
	if store.markCalls != 1 || queue.calls != 1 {
		t.Fatalf("mark calls = %d, queue calls = %d", store.markCalls, queue.calls)
	}
	if store.createParams.Region != "eu-north-1" || store.createParams.ExternalName != AWSExternalName(teamID) {
		t.Fatalf("unexpected create params: %#v", store.createParams)
	}
	if queue.command.TenantID != tenantID || queue.command.TeamID != teamID || queue.command.EventID == uuid.Nil {
		t.Fatalf("unexpected provisioning command: %#v", queue.command)
	}
}

func TestRequestProvisioningDoesNotDuplicateActiveTenant(t *testing.T) {
	teamID := uuid.New()
	tx := &fakeTransaction{}
	store := &fakeTenantStore{tx: tx, created: Tenant{
		ID: uuid.New(), TeamID: teamID, Provider: ProviderAWSSES, Region: "eu-north-1",
		ExternalName: AWSExternalName(teamID), Status: StatusActive,
	}}
	queue := &fakeProvisionQueue{}

	got, err := NewService(store, queue).RequestProvisioning(context.Background(), teamID, "eu-north-1")
	if err != nil {
		t.Fatalf("RequestProvisioning() error = %v", err)
	}
	if got.Status != StatusActive || !tx.committed {
		t.Fatalf("tenant = %#v, committed = %v", got, tx.committed)
	}
	if store.markCalls != 0 || queue.calls != 0 {
		t.Fatalf("mark calls = %d, queue calls = %d", store.markCalls, queue.calls)
	}
}

func TestRequestProvisioningRollsBackWhenOutboxFails(t *testing.T) {
	teamID := uuid.New()
	tenantID := uuid.New()
	tx := &fakeTransaction{}
	store := &fakeTenantStore{
		tx:      tx,
		created: Tenant{ID: tenantID, TeamID: teamID, Status: StatusPending},
		markResult: Tenant{
			ID: tenantID, TeamID: teamID, Provider: ProviderAWSSES, Region: "eu-north-1",
			ExternalName: AWSExternalName(teamID), Status: StatusProvisioning,
			SuppressionScope: SuppressionScopeTenant, ReputationPolicy: ReputationPolicyStandard,
		},
	}
	queueErr := errors.New("outbox unavailable")
	queue := &fakeProvisionQueue{err: queueErr}

	_, err := NewService(store, queue).RequestProvisioning(context.Background(), teamID, "eu-north-1")
	if !errors.Is(err, queueErr) {
		t.Fatalf("RequestProvisioning() error = %v, want %v", err, queueErr)
	}
	if tx.committed {
		t.Fatal("transaction committed after outbox failure")
	}
	if !tx.rolledBack {
		t.Fatal("transaction was not rolled back")
	}
}

func TestRequestProvisioningRejectsUnsupportedRegionBeforeTransaction(t *testing.T) {
	store := &fakeTenantStore{}
	_, err := NewService(store, &fakeProvisionQueue{}).RequestProvisioning(context.Background(), uuid.New(), "eu-west-1")
	if err == nil {
		t.Fatal("RequestProvisioning() error = nil, want unsupported region")
	}
	if store.tx != nil {
		t.Fatal("transaction began for unsupported region")
	}
}
