package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	minimumRetryAfter = time.Second
	maximumRetryAfter = 24 * time.Hour
)

type Processor struct {
	queue    ResultQueue
	client   HTTPClient
	policy   RetryPolicy
	workerID string
	now      func() time.Time
}

func NewProcessor(queue ResultQueue, client HTTPClient, policy RetryPolicy, workerID string) *Processor {
	return &Processor{
		queue:    queue,
		client:   client,
		policy:   policy,
		workerID: strings.TrimSpace(workerID),
		now:      time.Now,
	}
}

// Handler is retained as a source-compatible name while callers migrate to Processor.
type Handler = Processor

func NewHandler(queue ResultQueue, client HTTPClient, policy RetryPolicy, workerID string) *Processor {
	return NewProcessor(queue, client, policy, workerID)
}

func (processor *Processor) Handle(ctx context.Context, delivery ClaimedDelivery) error {
	if processor == nil || processor.queue == nil || processor.client == nil {
		return ErrProcessorNotConfigured
	}
	if processor.workerID == "" {
		return errors.New("webhook delivery worker id is required")
	}
	if delivery.ID == uuid.Nil || delivery.EventID == uuid.Nil || delivery.EndpointID == uuid.Nil {
		return ErrInvalidDelivery
	}
	if len(delivery.SigningSecret) == 0 {
		return processor.finishFailure(ctx, delivery, nil, nil, errors.New("webhook signing secret is empty"))
	}

	command, err := NewCommand(delivery)
	if err != nil {
		return processor.finishFailure(ctx, delivery, nil, nil, err)
	}
	body, err := command.Encode()
	if err != nil {
		return processor.finishFailure(ctx, delivery, nil, nil, fmt.Errorf("encode webhook payload: %w", err))
	}

	timestamp := processor.now().UTC().Unix()
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "Dugble-Webhooks/1.0")
	headers.Set("X-Dugble-Event", delivery.EventType)
	headers.Set("X-Dugble-Event-Id", delivery.EventID.String())
	headers.Set("X-Dugble-Delivery-Id", delivery.ID.String())
	headers.Set(SignatureHeader, Sign(delivery.SigningSecret, timestamp, body))

	response, err := processor.client.Post(ctx, delivery.URL, headers, body)
	if err != nil {
		if ctx.Err() != nil {
			_ = processor.queue.ReleaseClaim(context.WithoutCancel(ctx), delivery.ID, processor.workerID)
			return ctx.Err()
		}
		return processor.finishFailure(ctx, delivery, nil, nil, err)
	}
	status := int32(response.StatusCode)
	responseBody := response.Body
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return processor.queue.MarkSucceeded(ctx, delivery.ID, processor.workerID, status, &responseBody)
	}

	cause := fmt.Errorf("webhook endpoint returned HTTP %d", response.StatusCode)
	if !retryableStatus(response.StatusCode) {
		return processor.markFailed(ctx, delivery, &status, &responseBody, cause)
	}
	if response.StatusCode == http.StatusTooManyRequests {
		if _, retry := processor.policy.Next(int(delivery.AttemptCount), processor.now()); !retry {
			return processor.markFailed(ctx, delivery, &status, &responseBody, cause)
		}
		if retryAt, ok := retryAfter(response.Header.Get("Retry-After"), processor.now); ok {
			if err := processor.queue.ScheduleRetry(ctx, delivery.ID, processor.workerID, retryAt, &status, &responseBody, cause.Error()); err != nil {
				return errors.Join(cause, err)
			}
			return nil
		}
	}
	return processor.finishFailure(ctx, delivery, &status, &responseBody, cause)
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError
}

func (processor *Processor) finishFailure(ctx context.Context, delivery ClaimedDelivery, status *int32, body *string, cause error) error {
	nextAttempt, retry := processor.policy.Next(int(delivery.AttemptCount), processor.now())
	if retry {
		if err := processor.queue.ScheduleRetry(ctx, delivery.ID, processor.workerID, nextAttempt, status, body, cause.Error()); err != nil {
			return errors.Join(cause, err)
		}
		return nil
	}
	return processor.markFailed(ctx, delivery, status, body, cause)
}

func (processor *Processor) markFailed(ctx context.Context, delivery ClaimedDelivery, status *int32, body *string, cause error) error {
	if err := processor.queue.MarkFailed(ctx, delivery.ID, processor.workerID, status, body, cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return nil
}

func retryAfter(value string, now func() time.Time) (time.Time, bool) {
	current := now().UTC()
	var retryAt time.Time
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		retryAt = current.Add(time.Duration(seconds) * time.Second)
	} else {
		parsed, err := http.ParseTime(value)
		if err != nil || parsed.Before(current) {
			return time.Time{}, false
		}
		retryAt = parsed.UTC()
	}

	minimum := current.Add(minimumRetryAfter)
	maximum := current.Add(maximumRetryAfter)
	if retryAt.Before(minimum) {
		return minimum, true
	}
	if retryAt.After(maximum) {
		return maximum, true
	}
	return retryAt, true
}
