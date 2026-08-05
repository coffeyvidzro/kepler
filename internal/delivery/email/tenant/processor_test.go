package tenantprovision

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type processorStore struct {
	tenant      emailtenant.Tenant
	activeID    uuid.UUID
	externalID  string
	tenantARN   string
	failedID    uuid.UUID
	failedCause error
}

func (store *processorStore) Get(context.Context, uuid.UUID) (emailtenant.Tenant, error) {
	return store.tenant, nil
}

func (store *processorStore) MarkActive(_ context.Context, id uuid.UUID, externalID, tenantARN string) (emailtenant.Tenant, error) {
	store.activeID, store.externalID, store.tenantARN = id, externalID, tenantARN
	return store.tenant, nil
}

func (store *processorStore) MarkFailed(_ context.Context, id uuid.UUID, cause error) (emailtenant.Tenant, error) {
	store.failedID, store.failedCause = id, cause
	return store.tenant, nil
}

type processorProvider struct {
	request platformemail.TenantProvisionRequest
	result  platformemail.TenantProvisionResult
	calls   int
}

func (provider *processorProvider) ProvisionTenant(_ context.Context, request platformemail.TenantProvisionRequest) (platformemail.TenantProvisionResult, error) {
	provider.calls++
	provider.request = request
	return provider.result, nil
}

func tenantFixture() (emailtenant.Tenant, Command) {
	teamID, tenantID := uuid.New(), uuid.New()
	tenant := emailtenant.Tenant{
		ID: tenantID, TeamID: teamID, Provider: emailtenant.ProviderAWSSES,
		Region: "eu-north-1", ExternalName: "dugble-team", Status: emailtenant.StatusProvisioning,
		SuppressionScope: emailtenant.SuppressionScopeTenant,
		ReputationPolicy: emailtenant.ReputationPolicyStandard,
	}
	command := Command{
		EventID: uuid.New(), TenantID: tenantID, TeamID: teamID,
		Provider: tenant.Provider, Region: tenant.Region, ExternalName: tenant.ExternalName,
		SchemaVersion: 1,
	}
	return tenant, command
}

func TestProcessorProvisionsAndActivatesTenant(t *testing.T) {
	tenant, command := tenantFixture()
	store := &processorStore{tenant: tenant}
	provider := &processorProvider{result: platformemail.TenantProvisionResult{
		ExternalID: "provider-tenant-id",
		TenantARN:  "arn:aws:ses:eu-north-1:123:tenant/example",
	}}

	if err := NewProcessor(store, provider).Handle(context.Background(), command); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if provider.calls != 1 || provider.request.ExternalName != tenant.ExternalName {
		t.Fatalf("provider request = %#v, calls = %d", provider.request, provider.calls)
	}
	if store.activeID != tenant.ID || store.externalID != provider.result.ExternalID || store.tenantARN != provider.result.TenantARN {
		t.Fatalf("activation = (%s, %q, %q)", store.activeID, store.externalID, store.tenantARN)
	}
}

func TestProcessorSkipsActiveTenant(t *testing.T) {
	tenant, command := tenantFixture()
	tenant.Status = emailtenant.StatusActive
	provider := &processorProvider{}
	if err := NewProcessor(&processorStore{tenant: tenant}, provider).Handle(context.Background(), command); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestProcessorMarksExhaustedTenantFailed(t *testing.T) {
	tenant, command := tenantFixture()
	cause := errors.New("retries exhausted")
	store := &processorStore{tenant: tenant}
	if err := NewProcessor(store, nil).HandleExhausted(context.Background(), command, cause); err != nil {
		t.Fatalf("HandleExhausted() error = %v", err)
	}
	if store.failedID != tenant.ID || !errors.Is(store.failedCause, cause) {
		t.Fatalf("failed transition = (%s, %v)", store.failedID, store.failedCause)
	}
}
