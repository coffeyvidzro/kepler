package domainreconciliation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domainmodule "github.com/coffeyvidzro/dugble/server/internal/modules/domain"
)

type Processor struct {
	repository repository
	checker    checker
	config     Config
	workerID   string
	now        func() time.Time
}

func NewProcessor(repository repository, checker checker, config Config, workerID string) *Processor {
	return &Processor{
		repository: repository,
		checker:    checker,
		config:     config,
		workerID:   workerID,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (p *Processor) Process(ctx context.Context, claim domainmodule.ReconciliationClaim) error {
	if p == nil || p.repository == nil || p.checker == nil {
		return ErrConsumerNotConfigured
	}
	id, err := uuid.Parse(claim.Domain.ID)
	if err != nil {
		return fmt.Errorf("parse sender domain id: %w", err)
	}
	checkCtx, cancel := context.WithTimeout(ctx, p.config.CheckTimeout)
	defer cancel()
	result, checkErr := p.checker.Check(checkCtx, claim.Domain)
	if claim.Domain.Status == domainmodule.StatusVerified {
		return p.completeHealthCheck(ctx, id, result, checkErr)
	}

	delay := nextCheckDelay(max(claim.Attempt-1, 0), id)
	if checkErr == nil && result.Status == domainmodule.StatusVerified {
		delay = jitter(p.config.HealthCheckInterval, id)
	}
	nextCheckAt := p.now().Add(delay)
	if checkErr != nil {
		_, recordErr := p.repository.RecordReconciliationFailure(ctx, id, p.workerID, checkErr, nextCheckAt)
		return errors.Join(checkErr, recordErr)
	}
	_, err = p.repository.CompleteReconciliation(ctx, id, p.workerID, result.Status, result.VerificationRecords, nextCheckAt)
	return err
}

func (p *Processor) completeHealthCheck(ctx context.Context, id uuid.UUID, result domainmodule.ReconciliationResult, checkErr error) error {
	if checkErr == nil && result.Status == domainmodule.StatusVerified {
		_, err := p.repository.CompleteHealthCheck(ctx, id, p.workerID, p.now().Add(jitter(p.config.HealthCheckInterval, id)))
		return err
	}
	if checkErr == nil {
		checkErr = errors.New("sender domain verification checks no longer pass")
	}
	_, recordErr := p.repository.RecordHealthFailure(
		ctx,
		id,
		p.workerID,
		checkErr,
		p.config.HealthFailureThreshold,
		p.now().Add(jitter(p.config.HealthRetryInterval, id)),
	)
	return errors.Join(checkErr, recordErr)
}
