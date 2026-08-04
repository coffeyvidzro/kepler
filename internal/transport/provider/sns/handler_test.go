package sns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	awssns "github.com/coffeyvidzro/dugble/server/internal/integration/aws/sns"
)

type fakeVerifier struct{ err error }

func (f fakeVerifier) Verify(context.Context, awssns.Envelope) error { return f.err }

type fakeConfirmer struct {
	called   bool
	envelope awssns.Envelope
	err      error
}

func (f *fakeConfirmer) Confirm(_ context.Context, envelope awssns.Envelope) error {
	f.called = true
	f.envelope = envelope
	return f.err
}

type fakeIngestor struct {
	called   bool
	envelope awssns.Envelope
	err      error
}

func (f *fakeIngestor) Ingest(_ context.Context, envelope awssns.Envelope) error {
	f.called = true
	f.envelope = envelope
	return f.err
}

func TestReceiveSESConfirmsSubscription(t *testing.T) {
	confirmer := &fakeConfirmer{}
	handler := NewHandler(fakeVerifier{}, confirmer, &fakeIngestor{})
	response := performRequest(t, handler, subscriptionConfirmationBody())

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if !confirmer.called {
		t.Fatal("subscription confirmer was not called")
	}
}

func TestReceiveSESDispatchesNotification(t *testing.T) {
	ingestor := &fakeIngestor{}
	handler := NewHandler(fakeVerifier{}, &fakeConfirmer{}, ingestor)
	response := performRequest(t, handler, notificationBody())

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if !ingestor.called {
		t.Fatal("notification ingestor was not called")
	}
}

func TestReceiveSESRejectsInvalidJSON(t *testing.T) {
	handler := NewHandler(fakeVerifier{}, &fakeConfirmer{}, &fakeIngestor{})
	response := performRequest(t, handler, `{not-json`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestReceiveSESRejectsMismatchedMessageTypeHeader(t *testing.T) {
	handler := NewHandler(fakeVerifier{}, &fakeConfirmer{}, &fakeIngestor{})
	request := snsRequest(notificationBody())
	request.Header.Set("x-amz-sns-message-type", "SubscriptionConfirmation")
	response := serveSNSRequest(handler, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestReceiveSESRejectsMissingContentType(t *testing.T) {
	handler := NewHandler(fakeVerifier{}, &fakeConfirmer{}, &fakeIngestor{})
	request := snsRequest(notificationBody())
	request.Header.Del("Content-Type")
	response := serveSNSRequest(handler, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestReceiveSESMapsInvalidSignature(t *testing.T) {
	handler := NewHandler(fakeVerifier{err: awssns.ErrInvalidSignature}, &fakeConfirmer{}, &fakeIngestor{})
	response := performRequest(t, handler, notificationBody())
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

func TestReceiveSESReturnsUnavailableWhenIngestionFails(t *testing.T) {
	ingestor := &fakeIngestor{err: errors.New("database unavailable")}
	handler := NewHandler(fakeVerifier{}, &fakeConfirmer{}, ingestor)
	response := performRequest(t, handler, notificationBody())
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
}

func performRequest(t *testing.T, handler *Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveSNSRequest(handler, snsRequest(body))
}

func snsRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/integrations/aws/sns/ses", strings.NewReader(body))
	request.Header.Set("Content-Type", "text/plain; charset=UTF-8")
	if strings.Contains(body, `"Type":"SubscriptionConfirmation"`) {
		request.Header.Set("x-amz-sns-message-type", "SubscriptionConfirmation")
	} else {
		request.Header.Set("x-amz-sns-message-type", "Notification")
	}
	return request
}

func serveSNSRequest(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	e := echo.New()
	RegisterRoutes(e, handler)
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	return response
}

func notificationBody() string {
	return `{"Type":"Notification","MessageId":"message-id","TopicArn":"arn:aws:sns:us-east-1:123456789012:ses-events","Message":"{\"eventType\":\"Delivery\"}","Timestamp":"2026-07-31T08:00:00Z","SignatureVersion":"2","Signature":"signature","SigningCertURL":"https://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem"}`
}

func subscriptionConfirmationBody() string {
	return `{"Type":"SubscriptionConfirmation","MessageId":"message-id","TopicArn":"arn:aws:sns:us-east-1:123456789012:ses-events","Message":"confirm","Timestamp":"2026-07-31T08:00:00Z","SignatureVersion":"2","Signature":"signature","SigningCertURL":"https://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem","SubscribeURL":"https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription","Token":"token"}`
}
