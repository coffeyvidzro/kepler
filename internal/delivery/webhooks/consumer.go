package webhookdelivery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type claimStore interface {
	Claim(context.Context, string, int32, time.Time) ([]ClaimedDelivery, error)
}

type deliveryHandler interface {
	Handle(context.Context, ClaimedDelivery) error
}

type ConsumerConfig struct {
	PollInterval  time.Duration
	BatchSize     int32
	Concurrency   int
	LockTimeout   time.Duration
	HandleTimeout time.Duration
}

type Consumer struct {
	store    claimStore
	handler  deliveryHandler
	config   ConsumerConfig
	workerID string
}

func NewConsumer(store claimStore, handler deliveryHandler, config ConsumerConfig, workerID string) *Consumer {
	if config.PollInterval <= 0 {
		config.PollInterval = 500 * time.Millisecond
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}
	if config.Concurrency <= 0 {
		config.Concurrency = 10
	}
	if config.LockTimeout <= 0 {
		config.LockTimeout = 30 * time.Second
	}
	if config.HandleTimeout <= 0 {
		config.HandleTimeout = 15 * time.Second
	}
	return &Consumer{
		store: store, handler: handler, config: config,
		workerID: strings.TrimSpace(workerID),
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if c == nil || c.store == nil {
		return errors.New("webhook delivery claim store is not configured")
	}
	if c.handler == nil {
		return errors.New("webhook delivery handler is not configured")
	}
	if c.workerID == "" {
		return errors.New("webhook delivery worker id is required")
	}

	for {
		processed, err := c.processBatch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if processed == int(c.config.BatchSize) {
			continue
		}

		timer := time.NewTimer(c.config.PollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil
		case <-timer.C:
		}
	}
}

func (c *Consumer) processBatch(ctx context.Context) (int, error) {
	staleBefore := time.Now().UTC().Add(-c.config.LockTimeout)
	deliveries, err := c.store.Claim(ctx, c.workerID, c.config.BatchSize, staleBefore)
	if err != nil {
		return 0, fmt.Errorf("claim webhook deliveries: %w", err)
	}
	if len(deliveries) == 0 {
		return 0, nil
	}

	semaphore := make(chan struct{}, c.config.Concurrency)
	var group sync.WaitGroup

dispatchLoop:
	for _, delivery := range deliveries {
		if ctx.Err() != nil {
			break
		}
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			break dispatchLoop
		}
		if ctx.Err() != nil {
			<-semaphore
			break
		}
		group.Add(1)
		go func(delivery ClaimedDelivery) {
			defer group.Done()
			defer func() { <-semaphore }()

			handleContext, cancel := context.WithTimeout(ctx, c.config.HandleTimeout)
			defer cancel()
			if err := c.handler.Handle(handleContext, delivery); err != nil && ctx.Err() == nil {
				slog.Error(
					"webhook delivery failed",
					"delivery_id", delivery.ID,
					"event_id", delivery.EventID,
					"endpoint_id", delivery.EndpointID,
					"attempt", delivery.AttemptCount,
					"error", err,
				)
			}
		}(delivery)
	}
	group.Wait()
	return len(deliveries), ctx.Err()
}
