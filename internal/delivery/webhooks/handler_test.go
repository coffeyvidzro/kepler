package webhookdelivery

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeDeliveryStore struct {
	succeeded int
	retried   int
	failed    int
	retryAt   time.Time
}

func (s *fakeDeliveryStore) MarkSucceeded(context.Context, uuid.UUID, string, int32, *string) error {
	s.succeeded++
	return nil
}

func (s *fakeDeliveryStore) ScheduleRetry(_ context.Context, _ uuid.UUID, _ string, retryAt time.Time, _ *int32, _ *string, _ string) error {
	s.retried++
	s.retryAt = retryAt
	return nil
}

func (s *fakeDeliveryStore) MarkFailed(context.Context, uuid.UUID, string, *int32, *string, string) error {
	s.failed++
	return nil
}

func (s *fakeDeliveryStore) ReleaseClaim(context.Context, uuid.UUID, string) error {
	return nil
}

type fakeHTTPClient struct {
	response HTTPResponse
	err      error
}

func (c fakeHTTPClient) Post(context.Context, string, http.Header, []byte) (HTTPResponse, error) {
	return c.response, c.err
}

func testClaimedDelivery(attempt int32) ClaimedDelivery {
	return ClaimedDelivery{
		ID:            uuid.New(),
		EventID:       uuid.New(),
		EndpointID:    uuid.New(),
		AttemptCount:  attempt,
		TeamID:        uuid.New(),
		EventType:     "email.delivered",
		Payload:       []byte(`{"message_id":"test"}`),
		OccurredAt:    time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
		URL:           "https://example.com/webhooks",
		SigningSecret: []byte("secret"),
	}
}

func TestHandlerFailsPermanentClientErrorImmediately(t *testing.T) {
	store := &fakeDeliveryStore{}
	handler := NewHandler(store, fakeHTTPClient{response: HTTPResponse{StatusCode: http.StatusBadRequest}}, DefaultRetryPolicy(), "worker-1")

	if err := handler.Handle(context.Background(), testClaimedDelivery(1)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if store.failed != 1 || store.retried != 0 {
		t.Fatalf("failed = %d, retried = %d; want failed=1 retried=0", store.failed, store.retried)
	}
}

func TestHandlerRetriesServerError(t *testing.T) {
	store := &fakeDeliveryStore{}
	handler := NewHandler(store, fakeHTTPClient{response: HTTPResponse{StatusCode: http.StatusServiceUnavailable}}, RetryPolicy{Schedule: []time.Duration{time.Minute}}, "worker-1")
	fixedNow := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	handler.now = func() time.Time { return fixedNow }

	if err := handler.Handle(context.Background(), testClaimedDelivery(1)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if store.retried != 1 || store.failed != 0 {
		t.Fatalf("retried = %d, failed = %d; want retried=1 failed=0", store.retried, store.failed)
	}
	if want := fixedNow.Add(time.Minute); !store.retryAt.Equal(want) {
		t.Fatalf("retryAt = %v, want %v", store.retryAt, want)
	}
}

func TestHandlerClampsRetryAfterBounds(t *testing.T) {
	fixedNow := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		retryAfter string
		want       time.Time
	}{
		{name: "minimum", retryAfter: "0", want: fixedNow.Add(time.Second)},
		{name: "maximum", retryAfter: "999999", want: fixedNow.Add(24 * time.Hour)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeDeliveryStore{}
			header := make(http.Header)
			header.Set("Retry-After", tt.retryAfter)
			handler := NewHandler(store, fakeHTTPClient{response: HTTPResponse{
				StatusCode: http.StatusTooManyRequests,
				Header:     header,
			}}, RetryPolicy{Schedule: []time.Duration{time.Minute}}, "worker-1")
			handler.now = func() time.Time { return fixedNow }

			if err := handler.Handle(context.Background(), testClaimedDelivery(1)); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if store.retried != 1 || !store.retryAt.Equal(tt.want) {
				t.Fatalf("retried = %d, retryAt = %v; want retried=1 retryAt=%v", store.retried, store.retryAt, tt.want)
			}
		})
	}
}

func TestHandlerDoesNotRetry429AfterPolicyExhausted(t *testing.T) {
	store := &fakeDeliveryStore{}
	header := make(http.Header)
	header.Set("Retry-After", "30")
	handler := NewHandler(store, fakeHTTPClient{response: HTTPResponse{
		StatusCode: http.StatusTooManyRequests,
		Header:     header,
	}}, RetryPolicy{Schedule: []time.Duration{time.Minute}}, "worker-1")

	if err := handler.Handle(context.Background(), testClaimedDelivery(2)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if store.failed != 1 || store.retried != 0 {
		t.Fatalf("failed = %d, retried = %d; want failed=1 retried=0", store.failed, store.retried)
	}
}
