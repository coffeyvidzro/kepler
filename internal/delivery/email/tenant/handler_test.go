package tenantprovision

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	awsses "github.com/coffeyvidzro/dugble/server/internal/integration/aws/ses"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
)

type handlerStore struct {
	tenant      emailtenant.Tenant
	getErr      error
	activeID    uuid.UUID
	externalID  string
	tenantARN   string
	activeErr   error
	failedID    uuid.UUID
	failedCause error
	failedErr   error
}

func (s *handlerStore) Get(context.Context, uuid.UUID) (emailtenant.Tenant, error) {
	return s.tenant, s.getErr
}
func (s *handlerStore) MarkActive(_ context.Context, id uuid.UUID, externalID, tenantARN string) (emailtenant.Tenant, error) {
	s.activeID, s.externalID, s.tenantARN = id, externalID, tenantARN
	return s.tenant, s.activeErr
}
func (s *handlerStore) MarkFailed(_ context.Context, id uuid.UUID, cause error) (emailtenant.Tenant, error) {
	s.failedID, s.failedCause = id, cause
	return s.tenant, s.failedErr
}

type handlerProvider struct {
	request awsses.TenantProvisionRequest
	result  awsses.TenantProvisionResult
	err     error
	calls   int
}

func (p *handlerProvider) ProvisionTenant(_ context.Context, request awsses.TenantProvisionRequest) (awsses.TenantProvisionResult, error) {
	p.calls++
	p.request = request
	return p.result, p.err
}

func provisioningFixture() (emailtenant.Tenant, emailtenant.ProvisionCommand) {
	teamID, tenantID := uuid.New(), uuid.New()
	tenant := emailtenant.Tenant{ID: tenantID, TeamID: teamID, Provider: emailtenant.ProviderAWSSES, Region: "eu-north-1", ExternalName: "dugble-team", Status: emailtenant.StatusProvisioning, SuppressionScope: emailtenant.SuppressionScopeTenant, ReputationPolicy: emailtenant.ReputationPolicyStandard}
	command := emailtenant.ProvisionCommand{TenantID: tenantID, TeamID: teamID, Provider: tenant.Provider, Region: tenant.Region, ExternalName: tenant.ExternalName}
	return tenant, command
}

func TestHandlerProvisionsAndActivatesTenant(t *testing.T) {
	tenant, command := provisioningFixture()
	store := &handlerStore{tenant: tenant}
	provider := &handlerProvider{result: awsses.TenantProvisionResult{ExternalID: "provider-tenant-id", TenantARN: "arn:aws:ses:eu-north-1:123:tenant/example"}}

	if err := NewHandler(store, provider).Handle(context.Background(), command); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if provider.calls != 1 || provider.request.ExternalName != tenant.ExternalName || provider.request.Region != tenant.Region {
		t.Fatalf("provider request = %#v, calls = %d", provider.request, provider.calls)
	}
	if store.activeID != tenant.ID || store.externalID != "provider-tenant-id" || store.tenantARN != provider.result.TenantARN {
		t.Fatalf("activation = (%s, %q, %q)", store.activeID, store.externalID, store.tenantARN)
	}
}

func TestHandlerSkipsAlreadyActiveTenant(t *testing.T) {
	tenant, command := provisioningFixture()
	tenant.Status = emailtenant.StatusActive
	provider := &handlerProvider{}
	if err := NewHandler(&handlerStore{tenant: tenant}, provider).Handle(context.Background(), command); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestHandlerRejectsCommandThatDoesNotMatchTenant(t *testing.T) {
	tenant, command := provisioningFixture()
	command.TeamID = uuid.New()
	provider := &handlerProvider{}
	if err := NewHandler(&handlerStore{tenant: tenant}, provider).Handle(context.Background(), command); err == nil {
		t.Fatal("Handle() error = nil, want mismatch error")
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}
}

func TestHandlerReturnsProviderAndActivationFailures(t *testing.T) {
	tenant, command := provisioningFixture()
	providerErr := errors.New("SES unavailable")
	if err := NewHandler(&handlerStore{tenant: tenant}, &handlerProvider{err: providerErr}).Handle(context.Background(), command); !errors.Is(err, providerErr) {
		t.Fatalf("provider error = %v, want %v", err, providerErr)
	}

	activationErr := errors.New("database unavailable")
	if err := NewHandler(&handlerStore{tenant: tenant, activeErr: activationErr}, &handlerProvider{result: awsses.TenantProvisionResult{ExternalID: "external"}}).Handle(context.Background(), command); !errors.Is(err, activationErr) {
		t.Fatalf("activation error = %v, want %v", err, activationErr)
	}
}

func TestHandlerExhaustedMarksProvisioningTenantFailed(t *testing.T) {
	tenant, command := provisioningFixture()
	cause := errors.New("retries exhausted")
	store := &handlerStore{tenant: tenant}
	if err := NewHandler(store, nil).HandleExhausted(context.Background(), command, cause); err != nil {
		t.Fatalf("HandleExhausted() error = %v", err)
	}
	if store.failedID != tenant.ID || !errors.Is(store.failedCause, cause) {
		t.Fatalf("failed transition = (%s, %v)", store.failedID, store.failedCause)
	}
}

func TestHandlerExhaustedLeavesTerminalTenantUnchanged(t *testing.T) {
	for _, status := range []string{emailtenant.StatusActive, emailtenant.StatusFailed} {
		t.Run(status, func(t *testing.T) {
			tenant, command := provisioningFixture()
			tenant.Status = status
			store := &handlerStore{tenant: tenant}
			if err := NewHandler(store, nil).HandleExhausted(context.Background(), command, errors.New("ignored")); err != nil {
				t.Fatalf("HandleExhausted() error = %v", err)
			}
			if store.failedID != uuid.Nil {
				t.Fatalf("failed tenant = %s, want nil", store.failedID)
			}
		})
	}
}
