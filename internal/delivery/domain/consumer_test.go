package domainreconciliation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
)

type fakeRepository struct {
	claims          []domainmodule.ReconciliationClaim
	completed       bool
	failed          bool
	completedAs     string
	nextCheckAt     time.Time
	healthy         bool
	healthFailed    bool
	healthThreshold int32
}

func (r *fakeRepository) ClaimPendingReconciliations(context.Context, string, int32, time.Time) ([]domainmodule.ReconciliationClaim, error) {
	return r.claims, nil
}
func (r *fakeRepository) CompleteReconciliation(_ context.Context, _ uuid.UUID, _, status string, _ []domainmodule.VerificationRecord, next time.Time) (domainmodule.SenderDomain, error) {
	r.completed, r.completedAs, r.nextCheckAt = true, status, next
	return domainmodule.SenderDomain{}, nil
}
func (r *fakeRepository) RecordReconciliationFailure(_ context.Context, _ uuid.UUID, _ string, _ error, next time.Time) (domainmodule.SenderDomain, error) {
	r.failed, r.nextCheckAt = true, next
	return domainmodule.SenderDomain{}, nil
}
func (r *fakeRepository) CompleteHealthCheck(_ context.Context, _ uuid.UUID, _ string, next time.Time) (domainmodule.SenderDomain, error) {
	r.healthy, r.nextCheckAt = true, next
	return domainmodule.SenderDomain{}, nil
}
func (r *fakeRepository) RecordHealthFailure(_ context.Context, _ uuid.UUID, _ string, _ error, threshold int32, next time.Time) (domainmodule.SenderDomain, error) {
	r.healthFailed, r.healthThreshold, r.nextCheckAt = true, threshold, next
	return domainmodule.SenderDomain{}, nil
}

type fakeChecker struct {
	result domainmodule.ReconciliationResult
	err    error
}

var testConfig = Config{
	PollInterval: time.Second, BatchSize: 1, Concurrency: 1, LockTimeout: time.Minute, CheckTimeout: time.Second,
	HealthCheckInterval: 24 * time.Hour, HealthRetryInterval: time.Hour, HealthFailureThreshold: 3,
}

func (c fakeChecker) Check(context.Context, domainmodule.SenderDomain) (domainmodule.ReconciliationResult, error) {
	return c.result, c.err
}

func TestPollCompletesVerifiedReconciliation(t *testing.T) {
	id := uuid.MustParse("01000000-0000-0000-0000-000000000000")
	repository := &fakeRepository{claims: []domainmodule.ReconciliationClaim{{Domain: domainmodule.SenderDomain{ID: id.String()}, Attempt: 3}}}
	consumer := NewConsumer(repository, fakeChecker{result: domainmodule.ReconciliationResult{Status: domainmodule.StatusVerified}}, testConfig, "worker")
	now := time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC)
	consumer.now = func() time.Time { return now }

	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !repository.completed || repository.failed || repository.completedAs != domainmodule.StatusVerified {
		t.Fatalf("unexpected repository result: %+v", repository)
	}
	if !repository.nextCheckAt.After(now) {
		t.Fatalf("next check = %s, want after %s", repository.nextCheckAt, now)
	}
}

func TestPollRecordsTransientCheckFailure(t *testing.T) {
	id := uuid.MustParse("0a000000-0000-0000-0000-000000000000")
	repository := &fakeRepository{claims: []domainmodule.ReconciliationClaim{{Domain: domainmodule.SenderDomain{ID: id.String()}, Attempt: 2}}}
	consumer := NewConsumer(repository, fakeChecker{err: errors.New("SES unavailable")}, testConfig, "worker")
	now := time.Date(2026, time.July, 28, 0, 0, 0, 0, time.UTC)
	consumer.now = func() time.Time { return now }
	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !repository.failed || repository.completed {
		t.Fatalf("unexpected repository result: %+v", repository)
	}
	if want := now.Add(time.Minute); !repository.nextCheckAt.Equal(want) {
		t.Fatalf("next check = %s, want %s", repository.nextCheckAt, want)
	}
}

func TestVerifiedDomainHealthCheckBecomesHealthy(t *testing.T) {
	id := uuid.New()
	repository := &fakeRepository{claims: []domainmodule.ReconciliationClaim{{Domain: domainmodule.SenderDomain{ID: id.String(), Status: domainmodule.StatusVerified}}}}
	consumer := NewConsumer(repository, fakeChecker{result: domainmodule.ReconciliationResult{Status: domainmodule.StatusVerified}}, testConfig, "worker")
	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !repository.healthy || repository.healthFailed || repository.completed {
		t.Fatalf("unexpected repository result: %+v", repository)
	}
}

func TestVerifiedDomainHealthFailureDoesNotUseVerificationCompletion(t *testing.T) {
	id := uuid.New()
	repository := &fakeRepository{claims: []domainmodule.ReconciliationClaim{{Domain: domainmodule.SenderDomain{ID: id.String(), Status: domainmodule.StatusVerified}}}}
	consumer := NewConsumer(repository, fakeChecker{result: domainmodule.ReconciliationResult{Status: domainmodule.StatusPending}}, testConfig, "worker")
	if err := consumer.poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if !repository.healthFailed || repository.healthThreshold != 3 || repository.completed {
		t.Fatalf("unexpected repository result: %+v", repository)
	}
}

func TestNextCheckDelayIsBoundedAndJittered(t *testing.T) {
	id := uuid.MustParse("14000000-0000-0000-0000-000000000000")
	if got := nextCheckDelay(1, id); got <= 0 || got >= 2*time.Minute {
		t.Fatalf("first retry delay = %s", got)
	}
	if got := nextCheckDelay(100, id); got > 7*time.Hour {
		t.Fatalf("capped retry delay = %s", got)
	}
}
