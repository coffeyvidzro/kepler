package senderidreconciliation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	senderidmodule "github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

const (
	providerStatusSubmissionFailed  = "submission_failed"
	providerStatusSubmissionUnknown = "submission_unknown"
)

var (
	ErrConsumerNotConfigured = errors.New("Sender ID reconciliation consumer is not configured")
	ErrInvalidConfig         = errors.New("invalid Sender ID reconciliation configuration")
	ErrWorkerIDRequired      = errors.New("Sender ID reconciliation worker ID is required")
)

type repository interface {
	ClaimPendingRegistrations(context.Context, string, string, int32, time.Time) ([]senderidmodule.RegistrationClaim, error)
	CompleteSubmission(context.Context, uuid.UUID, string, string, time.Time) error
	CompleteStatus(context.Context, uuid.UUID, string, string, string, bool, *string, time.Time) error
	RecordProviderFailure(context.Context, uuid.UUID, string, string, error, time.Time) error
}

type Config struct {
	PollInterval        time.Duration
	BatchSize           int32
	Concurrency         int
	LockTimeout         time.Duration
	ProviderTimeout     time.Duration
	PendingCheckInterval time.Duration
	RetryBaseInterval   time.Duration
	MaxRetryInterval    time.Duration
}

func DefaultConfig() Config {
	return Config{
		PollInterval:         15 * time.Second,
		BatchSize:            25,
		Concurrency:          5,
		LockTimeout:          2 * time.Minute,
		ProviderTimeout:      20 * time.Second,
		PendingCheckInterval: 2 * time.Minute,
		RetryBaseInterval:    30 * time.Second,
		MaxRetryInterval:     time.Hour,
	}
}

func (config Config) validate() error {
	if config.PollInterval <= 0 || config.BatchSize <= 0 || config.Concurrency <= 0 ||
		config.LockTimeout <= 0 || config.ProviderTimeout <= 0 ||
		config.PendingCheckInterval <= 0 || config.RetryBaseInterval <= 0 ||
		config.MaxRetryInterval < config.RetryBaseInterval {
		return ErrInvalidConfig
	}
	return nil
}

type Consumer struct {
	repository repository
	providers  map[string]platformsenderid.Provider
	config     Config
	workerID   string
	now        func() time.Time
}

func NewConsumer(
	repository repository,
	config Config,
	workerID string,
	providers ...platformsenderid.Provider,
) (*Consumer, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" {
		return nil, ErrWorkerIDRequired
	}
	registry := make(map[string]platformsenderid.Provider, len(providers))
	for _, provider := range providers {
		if provider == nil {
			return nil, errors.New("Sender ID provider is required")
		}
		providerID := strings.ToLower(strings.TrimSpace(provider.ID()))
		if providerID == "" {
			return nil, errors.New("Sender ID provider ID is required")
		}
		if _, exists := registry[providerID]; exists {
			return nil, fmt.Errorf("duplicate Sender ID provider %q", providerID)
		}
		registry[providerID] = provider
	}
	if len(registry) == 0 {
		return nil, errors.New("at least one Sender ID provider is required")
	}
	return &Consumer{
		repository: repository,
		providers:  registry,
		config:     config,
		workerID:   workerID,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

func (consumer *Consumer) Run(ctx context.Context) error {
	if consumer == nil || consumer.repository == nil || len(consumer.providers) == 0 {
		return ErrConsumerNotConfigured
	}

	ticker := time.NewTicker(consumer.config.PollInterval)
	defer ticker.Stop()
	for {
		if err := consumer.poll(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Sender ID reconciliation poll failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type workItem struct {
	provider platformsenderid.Provider
	claim    senderidmodule.RegistrationClaim
}

func (consumer *Consumer) poll(ctx context.Context) error {
	now := consumer.now()
	items := make([]workItem, 0, int(consumer.config.BatchSize)*len(consumer.providers))
	var joined error
	for providerID, provider := range consumer.providers {
		claims, err := consumer.repository.ClaimPendingRegistrations(
			ctx,
			consumer.workerID,
			providerID,
			consumer.config.BatchSize,
			now.Add(-consumer.config.LockTimeout),
		)
		if err != nil {
			joined = errors.Join(joined, fmt.Errorf("claim %s Sender ID registrations: %w", providerID, err))
			continue
		}
		for _, claim := range claims {
			items = append(items, workItem{provider: provider, claim: claim})
		}
	}

	semaphore := make(chan struct{}, consumer.config.Concurrency)
	var wait sync.WaitGroup
	var mutex sync.Mutex
	for _, item := range items {
		item := item
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			if err := consumer.process(ctx, item.provider, item.claim); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error(
					"Sender ID reconciliation failed",
					"sender_id", item.claim.ID,
					"provider", item.claim.Provider,
					"attempt", item.claim.Attempt,
					"error", err,
				)
				mutex.Lock()
				joined = errors.Join(joined, err)
				mutex.Unlock()
			}
		}()
	}
	wait.Wait()
	return joined
}

func (consumer *Consumer) process(
	ctx context.Context,
	provider platformsenderid.Provider,
	claim senderidmodule.RegistrationClaim,
) error {
	if claim.ProviderSubmittedAt == nil && !strings.EqualFold(claim.ProviderStatus, providerStatusSubmissionUnknown) {
		return consumer.submit(ctx, provider, claim)
	}
	return consumer.checkStatus(ctx, provider, claim)
}

func (consumer *Consumer) submit(
	ctx context.Context,
	provider platformsenderid.Provider,
	claim senderidmodule.RegistrationClaim,
) error {
	providerCtx, cancel := context.WithTimeout(ctx, consumer.config.ProviderTimeout)
	response, err := provider.Create(providerCtx, platformsenderid.CreateRequest{SenderID: claim.Name})
	cancel()
	if err != nil {
		providerStatus := providerStatusSubmissionFailed
		if !definitiveProviderError(err) {
			providerStatus = providerStatusSubmissionUnknown
		}
		return consumer.recordFailure(ctx, claim, providerStatus, err)
	}
	if err := validateCreateResponse(provider, claim, response); err != nil {
		return consumer.recordFailure(ctx, claim, providerStatusSubmissionUnknown, err)
	}

	switch response.Status {
	case platformsenderid.StatusPending:
		return consumer.repository.CompleteSubmission(
			ctx,
			claim.ID,
			consumer.workerID,
			response.Status,
			consumer.now().Add(consumer.config.PendingCheckInterval),
		)
	case platformsenderid.StatusApproved, platformsenderid.StatusRejected:
		return consumer.completeStatus(ctx, claim, &platformsenderid.StatusResponse{
			ProviderID:     response.ProviderID,
			SenderID:       response.SenderID,
			Status:         response.Status,
			ProviderStatus: response.Status,
		})
	default:
		return consumer.recordFailure(
			ctx,
			claim,
			providerStatusSubmissionUnknown,
			fmt.Errorf("provider returned unknown Sender ID creation status %q", response.Status),
		)
	}
}

func (consumer *Consumer) checkStatus(
	ctx context.Context,
	provider platformsenderid.Provider,
	claim senderidmodule.RegistrationClaim,
) error {
	providerCtx, cancel := context.WithTimeout(ctx, consumer.config.ProviderTimeout)
	response, err := provider.CheckStatus(providerCtx, claim.Name)
	cancel()
	if err != nil {
		providerStatus := claim.ProviderStatus
		if providerStatus == "" {
			providerStatus = platformsenderid.StatusUnknown
		}
		return consumer.recordFailure(ctx, claim, providerStatus, err)
	}
	if err := validateStatusResponse(provider, claim, response); err != nil {
		return consumer.recordFailure(ctx, claim, platformsenderid.StatusUnknown, err)
	}
	return consumer.completeStatus(ctx, claim, response)
}

func (consumer *Consumer) completeStatus(
	ctx context.Context,
	claim senderidmodule.RegistrationClaim,
	response *platformsenderid.StatusResponse,
) error {
	var rejectionReason *string
	nextCheckAt := consumer.now()
	switch response.Status {
	case platformsenderid.StatusPending:
		nextCheckAt = nextCheckAt.Add(consumer.config.PendingCheckInterval)
	case platformsenderid.StatusApproved:
	case platformsenderid.StatusRejected:
		reason := "Sender ID was rejected by " + response.ProviderID
		rejectionReason = &reason
	default:
		return consumer.recordFailure(
			ctx,
			claim,
			response.ProviderStatus,
			fmt.Errorf("provider returned unknown Sender ID status %q", response.Status),
		)
	}
	return consumer.repository.CompleteStatus(
		ctx,
		claim.ID,
		consumer.workerID,
		response.Status,
		response.ProviderStatus,
		response.Whitelisted,
		rejectionReason,
		nextCheckAt,
	)
}

func (consumer *Consumer) recordFailure(
	ctx context.Context,
	claim senderidmodule.RegistrationClaim,
	providerStatus string,
	cause error,
) error {
	nextCheckAt := consumer.now().Add(consumer.retryDelay(claim.Attempt))
	recordErr := consumer.repository.RecordProviderFailure(
		ctx,
		claim.ID,
		consumer.workerID,
		providerStatus,
		cause,
		nextCheckAt,
	)
	return errors.Join(cause, recordErr)
}

func (consumer *Consumer) retryDelay(attempt int32) time.Duration {
	delay := consumer.config.RetryBaseInterval
	for current := int32(1); current < attempt && delay < consumer.config.MaxRetryInterval; current++ {
		if delay > consumer.config.MaxRetryInterval/2 {
			return consumer.config.MaxRetryInterval
		}
		delay *= 2
	}
	if delay > consumer.config.MaxRetryInterval {
		return consumer.config.MaxRetryInterval
	}
	return delay
}

func validateCreateResponse(
	provider platformsenderid.Provider,
	claim senderidmodule.RegistrationClaim,
	response *platformsenderid.CreateResponse,
) error {
	if response == nil {
		return errors.New("Sender ID provider returned an empty creation response")
	}
	if !strings.EqualFold(strings.TrimSpace(response.ProviderID), strings.TrimSpace(provider.ID())) {
		return fmt.Errorf("Sender ID provider response ID %q does not match %q", response.ProviderID, provider.ID())
	}
	if !strings.EqualFold(strings.TrimSpace(response.SenderID), strings.TrimSpace(claim.Name)) {
		return fmt.Errorf("Sender ID provider response name %q does not match %q", response.SenderID, claim.Name)
	}
	return nil
}

func validateStatusResponse(
	provider platformsenderid.Provider,
	claim senderidmodule.RegistrationClaim,
	response *platformsenderid.StatusResponse,
) error {
	if response == nil {
		return errors.New("Sender ID provider returned an empty status response")
	}
	if !strings.EqualFold(strings.TrimSpace(response.ProviderID), strings.TrimSpace(provider.ID())) {
		return fmt.Errorf("Sender ID provider response ID %q does not match %q", response.ProviderID, provider.ID())
	}
	if !strings.EqualFold(strings.TrimSpace(response.SenderID), strings.TrimSpace(claim.Name)) {
		return fmt.Errorf("Sender ID provider response name %q does not match %q", response.SenderID, claim.Name)
	}
	return nil
}

type safeFallbackError interface {
	error
	SafeToFallback() bool
}

func definitiveProviderError(err error) bool {
	var definitive safeFallbackError
	return errors.As(err, &definitive) && definitive.SafeToFallback()
}
