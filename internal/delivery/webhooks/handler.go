package webhookdelivery

import (
	"context"
	"encoding/json"
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

type ClaimedDelivery struct {
	ID            uuid.UUID
	EventID       uuid.UUID
	EndpointID    uuid.UUID
	AttemptCount  int32
	TeamID        uuid.UUID
	EventType     string
	Payload       json.RawMessage
	OccurredAt    time.Time
	URL           string
	SigningSecret []byte
}

type deliveryResultStore interface {
	MarkSucceeded(context.Context, uuid.UUID, string, int32, *string) error
	ScheduleRetry(context.Context, uuid.UUID, string, time.Time, *int32, *string, string) error
	MarkFailed(context.Context, uuid.UUID, string, *int32, *string, string) error
	ReleaseClaim(context.Context, uuid.UUID, string) error
}

type Handler struct {
	store    deliveryResultStore
	client   HTTPClient
	policy   RetryPolicy
	workerID string
	now      func() time.Time
}

func NewHandler(store deliveryResultStore, client HTTPClient, policy RetryPolicy, workerID string) *Handler {
	return &Handler{store: store, client: client, policy: policy, workerID: strings.TrimSpace(workerID), now: time.Now}
}

func (h *Handler) Handle(ctx context.Context, delivery ClaimedDelivery) error {
	if h == nil || h.store == nil || h.client == nil {
		return errors.New("webhook delivery handler is not configured")
	}
	if h.workerID == "" {
		return errors.New("webhook delivery worker id is required")
	}
	if delivery.ID == uuid.Nil || delivery.EventID == uuid.Nil || delivery.EndpointID == uuid.Nil {
		return errors.New("webhook delivery requires delivery, event, and endpoint IDs")
	}
	if len(delivery.SigningSecret) == 0 {
		return h.finishFailure(ctx, delivery, nil, nil, errors.New("webhook signing secret is empty"))
	}

	body, err := json.Marshal(struct {
		ID         string          `json:"id"`
		Type       string          `json:"type"`
		OccurredAt time.Time       `json:"occurred_at"`
		Data       json.RawMessage `json:"data"`
	}{
		ID:         delivery.EventID.String(),
		Type:       delivery.EventType,
		OccurredAt: delivery.OccurredAt.UTC(),
		Data:       delivery.Payload,
	})
	if err != nil {
		return h.finishFailure(ctx, delivery, nil, nil, fmt.Errorf("encode webhook payload: %w", err))
	}

	timestamp := h.now().UTC().Unix()
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "Dugble-Webhooks/1.0")
	headers.Set("X-Dugble-Event", delivery.EventType)
	headers.Set("X-Dugble-Event-Id", delivery.EventID.String())
	headers.Set("X-Dugble-Delivery-Id", delivery.ID.String())
	headers.Set(SignatureHeader, Sign(delivery.SigningSecret, timestamp, body))

	response, err := h.client.Post(ctx, delivery.URL, headers, body)
	if err != nil {
		if ctx.Err() != nil {
			_ = h.store.ReleaseClaim(context.WithoutCancel(ctx), delivery.ID, h.workerID)
			return ctx.Err()
		}
		return h.finishFailure(ctx, delivery, nil, nil, err)
	}
	status := int32(response.StatusCode)
	responseBody := response.Body
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return h.store.MarkSucceeded(ctx, delivery.ID, h.workerID, status, &responseBody)
	}

	cause := fmt.Errorf("webhook endpoint returned HTTP %d", response.StatusCode)
	if !retryableStatus(response.StatusCode) {
		return h.markFailed(ctx, delivery, &status, &responseBody, cause)
	}

	if response.StatusCode == http.StatusTooManyRequests {
		if _, retry := h.policy.Next(int(delivery.AttemptCount), h.now()); !retry {
			return h.markFailed(ctx, delivery, &status, &responseBody, cause)
		}
		if retryAt, ok := retryAfter(response.Header.Get("Retry-After"), h.now); ok {
			if err := h.store.ScheduleRetry(ctx, delivery.ID, h.workerID, retryAt, &status, &responseBody, cause.Error()); err != nil {
				return errors.Join(cause, err)
			}
			return nil
		}
	}
	return h.finishFailure(ctx, delivery, &status, &responseBody, cause)
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooEarly ||
		status == http.StatusTooManyRequests ||
		status >= http.StatusInternalServerError
}

func (h *Handler) finishFailure(ctx context.Context, delivery ClaimedDelivery, status *int32, body *string, cause error) error {
	nextAttempt, retry := h.policy.Next(int(delivery.AttemptCount), h.now())
	if retry {
		if err := h.store.ScheduleRetry(ctx, delivery.ID, h.workerID, nextAttempt, status, body, cause.Error()); err != nil {
			return errors.Join(cause, err)
		}
		return nil
	}
	return h.markFailed(ctx, delivery, status, body, cause)
}

func (h *Handler) markFailed(ctx context.Context, delivery ClaimedDelivery, status *int32, body *string, cause error) error {
	if err := h.store.MarkFailed(ctx, delivery.ID, h.workerID, status, body, cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return nil
}

func retryAfter(value string, now func() time.Time) (time.Time, bool) {
	current := now().UTC()
	var retryAt time.Time
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
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
