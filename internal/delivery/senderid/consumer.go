package senderidreconciliation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

type Config struct {
	PollInterval         time.Duration
	BatchSize            int32
	Concurrency          int
	LockTimeout          time.Duration
	ProviderTimeout      time.Duration
	PendingCheckInterval time.Duration
	RetryBaseInterval    time.Duration
	MaxRetryInterval     time.Duration
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
	repository registrationRepository
	providers  map[string]platformsenderid.Provider
	config     Config
	workerID   string
	now        func() time.Time
}

func NewConsumer(
	repository registrationRepository,
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
			return nil, errors.New("sender ID provider is required")
		}
		providerID := strings.ToLower(strings.TrimSpace(provider.ID()))
		if providerID == "" {
			return nil, errors.New("sender ID provider ID is required")
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
	claim    RegistrationClaim
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
