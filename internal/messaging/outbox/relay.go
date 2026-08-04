package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxStoredErrorLength = 2048

type Store interface {
	ClaimBatch(context.Context, string, int, time.Time) ([]Event, error)
	MarkPublished(context.Context, uuid.UUID, string) error
	Release(context.Context, uuid.UUID, string, time.Time, string) error
}

type Publisher interface {
	Publish(context.Context, string, []byte, map[string]string, string) error
}

type Config struct {
	PollInterval time.Duration
	BatchSize    int
	LockTimeout  time.Duration
}

type Relay struct {
	store     Store
	publisher Publisher
	config    Config
	workerID  string
}

func NewRelay(store Store, publisher Publisher, config Config) *Relay {
	if config.PollInterval <= 0 {
		config.PollInterval = 500 * time.Millisecond
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}
	if config.LockTimeout <= 0 {
		config.LockTimeout = 30 * time.Second
	}

	return &Relay{
		store:     store,
		publisher: publisher,
		config:    config,
		workerID:  "outbox-" + uuid.NewString(),
	}
}

func (r *Relay) Run(ctx context.Context) error {
	if r == nil || r.store == nil {
		return errors.New("outbox relay store is not configured")
	}
	if r.publisher == nil {
		return errors.New("outbox relay publisher is not configured")
	}

	for {
		processed, err := r.processBatch(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		if processed == r.config.BatchSize {
			continue
		}

		timer := time.NewTimer(r.config.PollInterval)
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

func (r *Relay) processBatch(ctx context.Context) (int, error) {
	staleBefore := time.Now().UTC().Add(-r.config.LockTimeout)
	events, err := r.store.ClaimBatch(ctx, r.workerID, r.config.BatchSize, staleBefore)
	if err != nil {
		return 0, fmt.Errorf("claim outbox batch: %w", err)
	}

	for _, event := range events {
		if err := r.processEvent(ctx, event); err != nil {
			return 0, err
		}
	}

	return len(events), nil
}

func (r *Relay) processEvent(ctx context.Context, event Event) error {
	headers := make(map[string]string, len(event.Headers)+3)
	for key, value := range event.Headers {
		headers[key] = value
	}
	headers["Dugble-Event-Id"] = event.ID.String()
	headers["Dugble-Aggregate-Type"] = event.AggregateType
	headers["Dugble-Aggregate-Id"] = event.AggregateID.String()

	publishErr := r.publisher.Publish(ctx, event.Subject, event.Payload, headers, event.ID.String())
	if publishErr == nil {
		if err := r.store.MarkPublished(ctx, event.ID, r.workerID); err != nil {
			return fmt.Errorf("complete outbox event %s: %w", event.ID, err)
		}
		return nil
	}

	nextAttempt := time.Now().UTC().Add(retryBackoff(event.Attempts))
	if err := r.store.Release(
		ctx,
		event.ID,
		r.workerID,
		nextAttempt,
		truncateError(publishErr),
	); err != nil {
		return errors.Join(fmt.Errorf("publish outbox event %s: %w", event.ID, publishErr), err)
	}

	slog.Warn(
		"outbox event publish failed",
		"event_id", event.ID,
		"subject", event.Subject,
		"attempt", event.Attempts,
		"next_attempt", nextAttempt,
		"error", publishErr,
	)
	return nil
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exponent := min(attempt-1, 9)
	seconds := math.Pow(2, float64(exponent))
	backoff := time.Duration(seconds) * time.Second
	if backoff > 15*time.Minute {
		return 15 * time.Minute
	}
	return backoff
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) <= maxStoredErrorLength {
		return message
	}
	return message[:maxStoredErrorLength]
}
