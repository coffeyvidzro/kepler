package senderidreconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	senderidmodule "github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

type fakeRepository struct {
	submitted      bool
	completedStatus string
	failureStatus  string
}

func (repository *fakeRepository) ClaimPendingRegistrations(context.Context, string, string, int32, time.Time) ([]senderidmodule.RegistrationClaim, error) {
	return nil, nil
}

func (repository *fakeRepository) CompleteSubmission(context.Context, uuid.UUID, string, string, time.Time) error {
	repository.submitted = true
	return nil
}

func (repository *fakeRepository) CompleteStatus(_ context.Context, _ uuid.UUID, _ string, status string, _ string, _ bool, _ *string, _ time.Time) error {
	repository.completedStatus = status
	return nil
}

func (repository *fakeRepository) RecordProviderFailure(_ context.Context, _ uuid.UUID, _ string, providerStatus string, _ error, _ time.Time) error {
	repository.failureStatus = providerStatus
	return nil
}

type fakeProvider struct {
	createResponse *platformsenderid.CreateResponse
	createError    error
	statusResponse *platformsenderid.StatusResponse
	statusError    error
	createCalls    int
	statusCalls    int
}

func (provider *fakeProvider) ID() string { return platformsenderid.ProviderMoolre }

func (provider *fakeProvider) Create(context.Context, platformsenderid.CreateRequest) (*platformsenderid.CreateResponse, error) {
	provider.createCalls++
	return provider.createResponse, provider.createError
}

func (provider *fakeProvider) CheckStatus(context.Context, string) (*platformsenderid.StatusResponse, error) {
	provider.statusCalls++
	return provider.statusResponse, provider.statusError
}

func newTestConsumer(t *testing.T, repository *fakeRepository, provider *fakeProvider) *Consumer {
	t.Helper()
	consumer, err := NewConsumer(repository, DefaultConfig(), "worker-1", provider)
	if err != nil {
		t.Fatalf("NewConsumer() error = %v", err)
	}
	consumer.now = func() time.Time { return time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC) }
	return consumer
}

func TestProcessSubmitsNewSenderID(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	provider := &fakeProvider{createResponse: &platformsenderid.CreateResponse{
		ProviderID: platformsenderid.ProviderMoolre,
		SenderID:   "Dugble1",
		Status:     platformsenderid.StatusPending,
	}}
	consumer := newTestConsumer(t, repository, provider)
	claim := senderidmodule.RegistrationClaim{
		ID:       uuid.New(),
		Name:     "Dugble1",
		Provider: platformsenderid.ProviderMoolre,
		Attempt:  1,
	}

	if err := consumer.process(context.Background(), provider, claim); err != nil {
		t.Fatalf("process() error = %v", err)
	}
	if !repository.submitted || provider.createCalls != 1 || provider.statusCalls != 0 {
		t.Fatalf("submitted=%v createCalls=%d statusCalls=%d", repository.submitted, provider.createCalls, provider.statusCalls)
	}
}

func TestProcessApprovesSubmittedSenderID(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	provider := &fakeProvider{statusResponse: &platformsenderid.StatusResponse{
		ProviderID:     platformsenderid.ProviderMoolre,
		SenderID:       "Dugble1",
		Status:         platformsenderid.StatusApproved,
		ProviderStatus: "Approved",
	}}
	consumer := newTestConsumer(t, repository, provider)
	submittedAt := time.Now()
	claim := senderidmodule.RegistrationClaim{
		ID:                  uuid.New(),
		Name:                "Dugble1",
		Provider:            platformsenderid.ProviderMoolre,
		ProviderSubmittedAt: &submittedAt,
		Attempt:             2,
	}

	if err := consumer.process(context.Background(), provider, claim); err != nil {
		t.Fatalf("process() error = %v", err)
	}
	if repository.completedStatus != platformsenderid.StatusApproved {
		t.Fatalf("completed status = %q", repository.completedStatus)
	}
}

func TestUncertainSubmissionIsResolvedByStatusCheck(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	provider := &fakeProvider{createError: errors.New("connection reset")}
	consumer := newTestConsumer(t, repository, provider)
	claim := senderidmodule.RegistrationClaim{
		ID:       uuid.New(),
		Name:     "Dugble1",
		Provider: platformsenderid.ProviderMoolre,
		Attempt:  1,
	}

	if err := consumer.process(context.Background(), provider, claim); err == nil {
		t.Fatal("process() error = nil")
	}
	if repository.failureStatus != providerStatusSubmissionUnknown {
		t.Fatalf("failure status = %q", repository.failureStatus)
	}

	provider.createError = nil
	provider.statusResponse = &platformsenderid.StatusResponse{
		ProviderID:     platformsenderid.ProviderMoolre,
		SenderID:       "Dugble1",
		Status:         platformsenderid.StatusPending,
		ProviderStatus: "Pending",
	}
	claim.ProviderStatus = providerStatusSubmissionUnknown
	claim.Attempt = 2
	if err := consumer.process(context.Background(), provider, claim); err != nil {
		t.Fatalf("resolve process() error = %v", err)
	}
	if provider.createCalls != 1 || provider.statusCalls != 1 {
		t.Fatalf("createCalls=%d statusCalls=%d", provider.createCalls, provider.statusCalls)
	}
}
