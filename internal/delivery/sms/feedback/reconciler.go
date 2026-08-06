package feedback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type pendingRepository interface {
	ListPending(context.Context, int32) ([]PendingMessage, error)
}

type statusChecker interface {
	CheckStatus(context.Context, string, string) (*smsapi.StatusResponse, error)
}

type ReconcilerConfig struct {
	BatchSize       int32
	Concurrency     int
	ProviderTimeout time.Duration
}

type Reconciler struct {
	repository pendingRepository
	checker    statusChecker
	processor  *Processor
	config     ReconcilerConfig
	now        func() time.Time
}

func NewReconciler(repository pendingRepository, checker statusChecker, processor *Processor, config ReconcilerConfig) *Reconciler {
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 10
	}
	if config.ProviderTimeout <= 0 {
		config.ProviderTimeout = 15 * time.Second
	}
	return &Reconciler{
		repository: repository,
		checker:    checker,
		processor:  processor,
		config:     config,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (reconciler *Reconciler) ReconcileBatch(ctx context.Context) (int, error) {
	if reconciler == nil || reconciler.repository == nil || reconciler.checker == nil || reconciler.processor == nil {
		return 0, ErrReconcilerNotConfigured
	}
	messages, err := reconciler.repository.ListPending(ctx, reconciler.config.BatchSize)
	if err != nil {
		return 0, err
	}
	if len(messages) == 0 {
		return 0, nil
	}

	semaphore := make(chan struct{}, reconciler.config.Concurrency)
	var wait sync.WaitGroup
	var mutex sync.Mutex
	var joined error
	processed := 0
	for _, message := range messages {
		message := message
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				mutex.Lock()
				joined = errors.Join(joined, ctx.Err())
				mutex.Unlock()
				return
			}

			providerCtx, cancel := context.WithTimeout(ctx, reconciler.config.ProviderTimeout)
			response, checkErr := reconciler.checker.CheckStatus(
				providerCtx,
				message.ProviderID,
				message.ProviderMessageID,
			)
			cancel()
			if checkErr != nil {
				mutex.Lock()
				joined = errors.Join(joined, fmt.Errorf("check %s SMS %s status: %w", message.ProviderID, message.ProviderMessageID, checkErr))
				mutex.Unlock()
				return
			}
			event, eventErr := statusEvent(message, response, reconciler.now())
			if eventErr != nil {
				mutex.Lock()
				joined = errors.Join(joined, eventErr)
				mutex.Unlock()
				return
			}
			if _, processErr := reconciler.processor.Handle(ctx, event); processErr != nil {
				mutex.Lock()
				joined = errors.Join(joined, processErr)
				mutex.Unlock()
				return
			}
			mutex.Lock()
			processed++
			mutex.Unlock()
		}()
	}
	wait.Wait()
	return processed, joined
}
