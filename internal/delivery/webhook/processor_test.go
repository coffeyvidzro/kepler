package webhook

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeResultQueue struct {
	succeeded int
	retried   int
	failed    int
	retryAt   time.Time
}

func (queue *fakeResultQueue) MarkSucceeded(context.Context, uuid.UUID, string, int32, *string) error {
	queue.succeeded++
	return nil
}

func (queue *fakeResultQueue) ScheduleRetry(_ context.Context, _ uuid.UUID, _ string, retryAt time.Time, _ *int32, _ *string, _ string) error {
	queue.retried++
	queue.retryAt = retryAt
	return nil
}

func (queue *fakeResultQueue) MarkFailed(context.Context, uuid.UUID, string, *int32, *string, string) error {
	queue.failed++
	return nil
}

func (queue *fakeResultQueue) ReleaseClaim(context.Context, uuid.UUID, string) error { return nil }

type fakeHTTPClient struct {
	response HTTPResponse
	err      error
}

func (client fakeHTTPClient) Post(context.Context, string, http.Header, []byte) (HTTPResponse, error) {
	return client.response, client.err
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

func TestProcessorFailsPermanentClientErrorImmediately(t *testing.T) {
	queue := &fakeResultQueue{}
	processor := NewProcessor(queue, fakeHTTPClient{response: HTTPResponse{StatusCode: http.StatusBadRequest}}, DefaultRetryPolicy(), "worker-1")

	if err := processor.Handle(context.Background(), testClaimedDelivery(1)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if queue.failed != 1 || queue.retried != 0 {
		t.Fatalf("failed = %d, retried = %d; want failed=1 retried=0", queue.failed, queue.retried)
	}
}

func TestProcessorRetriesServerError(t *testing.T) {
	queue := &fakeResultQueue{}
	processor := NewProcessor(queue, fakeHTTPClient{response: HTTPResponse{StatusCode: http.StatusServiceUnavailable}}, RetryPolicy{Schedule: []time.Duration{time.Minute}}, "worker-1")
	fixedNow := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	processor.now = func() time.Time { return fixedNow }

	if err := processor.Handle(context.Background(), testClaimedDelivery(1)); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if queue.retried != 1 || queue.failed != 0 {
		t.Fatalf("retried = %d, failed = %d; want retried=1 failed=0", queue.retried, queue.failed)
	}
	if want := fixedNow.Add(time.Minute); !queue.retryAt.Equal(want) {
		t.Fatalf("retryAt = %v, want %v", queue.retryAt, want)
	}
}

func TestProcessorClampsRetryAfterBounds(t *testing.T) {
	fixedNow := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		retryAfter string
		want       time.Time
	}{
		{name: "minimum", retryAfter: "0", want: fixedNow.Add(time.Second)},
		{name: "maximum", retryAfter: "999999", want: fixedNow.Add(24 * time.Hour)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queue := &fakeResultQueue{}
			headers := make(http.Header)
			headers.Set("Retry-After", test.retryAfter)
			processor := NewProcessor(queue, fakeHTTPClient{response: HTTPResponse{
				StatusCode: http.StatusTooManyRequests,
				Header:     headers,
			}}, RetryPolicy{Schedule: []time.Duration{time.Minute}}, "worker-1")
			processor.now = func() time.Time { return fixedNow }

			if err := processor.Handle(context.Background(), testClaimedDelivery(1)); err != nil {
				t.Fatalf("Handle() error = %v", err)
			}
			if queue.retried != 1 || !queue.retryAt.Equal(test.want) {
				t.Fatalf("retried = %d, retryAt = %v; want retried=1 retryAt=%v", queue.retried, queue.retryAt, test.want)
			}
		})
	}
}
