package sms

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func TestNewServiceRequiresBillingAuthorizer(t *testing.T) {
	billing := platformbilling.NewService(nil)
	service := NewService(nil, nil, nil, billing)
	if service.billing != billing {
		t.Fatal("NewService did not retain the required billing authorizer")
	}
}

func TestSMSBillingErrorMapping(t *testing.T) {
	tests := []struct {
		name   string
		source error
		code   string
		status int
	}{
		{name: "insufficient balance", source: platformbilling.ErrInsufficientBalance, code: "PAYMENT_REQUIRED", status: http.StatusPaymentRequired},
		{name: "invalid destination", source: platformbilling.ErrInvalidDestination, code: "BAD_REQUEST", status: http.StatusBadRequest},
		{name: "invalid segments", source: platformbilling.ErrInvalidSegments, code: "BAD_REQUEST", status: http.StatusBadRequest},
		{name: "team not found", source: platformbilling.ErrTeamNotFound, code: "NOT_FOUND", status: http.StatusNotFound},
		{name: "inactive team", source: platformbilling.ErrTeamInactive, code: "CONFLICT", status: http.StatusConflict},
		{name: "unsupported market", source: platformbilling.ErrUnsupportedMarket, code: "CONFLICT", status: http.StatusConflict},
		{name: "wallet not found", source: platformbilling.ErrWalletNotFound, code: "CONFLICT", status: http.StatusConflict},
		{name: "rate not found", source: platformbilling.ErrRateNotFound, code: "SERVICE_UNAVAILABLE", status: http.StatusServiceUnavailable},
		{name: "currency mismatch", source: platformbilling.ErrCurrencyMismatch, code: "CONFLICT", status: http.StatusConflict},
		{name: "amount overflow", source: platformbilling.ErrAmountOverflow, code: "INTERNAL_ERROR", status: http.StatusInternalServerError},
		{name: "invalid team id", source: platformbilling.ErrInvalidTeamID, code: "INTERNAL_ERROR", status: http.StatusInternalServerError},
		{name: "invalid message id", source: platformbilling.ErrInvalidMessageID, code: "INTERNAL_ERROR", status: http.StatusInternalServerError},
		{name: "unknown", source: errors.New("unknown billing error"), code: "INTERNAL_ERROR", status: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped := smsBillingError(fmt.Errorf("authorization failed: %w", test.source))
			var appErr *apperrors.AppError
			if !errors.As(mapped, &appErr) {
				t.Fatalf("smsBillingError() = %T, want *errors.AppError", mapped)
			}
			if appErr.Code != test.code || appErr.Status != test.status {
				t.Fatalf("smsBillingError() = (%s, %d), want (%s, %d)", appErr.Code, appErr.Status, test.code, test.status)
			}
		})
	}
}

func TestValidateSendRequiresE164Recipient(t *testing.T) {
	_, err := validateSend(SendRequest{To: "0241234567", From: "DUGBLE", Body: "hello"})
	if err == nil {
		t.Fatal("validateSend returned nil error for non-E.164 recipient")
	}
}

func TestValidateSendDefaultsMetadata(t *testing.T) {
	req, err := validateSend(SendRequest{To: "+233241234567", From: "DUGBLE", Body: "hello"})
	if err != nil {
		t.Fatalf("validateSend returned error: %v", err)
	}
	if string(req.Metadata) != "{}" {
		t.Fatalf("Metadata = %s, want {}", req.Metadata)
	}
}

func TestValidateSendResolvesDestinationCountry(t *testing.T) {
	req, err := validateSend(SendRequest{To: "+233241234567", From: "DUGBLE", Body: "hello"})
	if err != nil {
		t.Fatalf("validateSend returned error: %v", err)
	}
	if req.DestinationCountry != "GH" {
		t.Fatalf("DestinationCountry = %q, want GH", req.DestinationCountry)
	}
}

func TestValidateSendResolvesKenyaDestination(t *testing.T) {
	req, err := validateSend(SendRequest{To: "+254712345678", From: "DUGBLE", Body: "hello"})
	if err != nil {
		t.Fatalf("validateSend returned error: %v", err)
	}
	if req.DestinationCountry != "KE" {
		t.Fatalf("DestinationCountry = %q, want KE", req.DestinationCountry)
	}
}

func TestValidateSendRejectsUnsupportedDestination(t *testing.T) {
	_, err := validateSend(SendRequest{To: "+12025550123", From: "DUGBLE", Body: "hello"})
	if err == nil {
		t.Fatal("validateSend returned nil error for unsupported destination")
	}
}

func TestValidateBatchSendRequiresMessages(t *testing.T) {
	if err := validateBatchSend(BatchSendRequest{}); err == nil {
		t.Fatal("validateBatchSend returned nil error for empty batch")
	}
}

func TestValidateBatchSendLimitsMessages(t *testing.T) {
	messages := make([]SendRequest, maxBatchMessages+1)
	for i := range messages {
		messages[i] = SendRequest{To: "+233241234567", From: "DUGBLE", Body: "hello"}
	}
	if err := validateBatchSend(BatchSendRequest{Messages: messages}); err == nil {
		t.Fatal("validateBatchSend returned nil error for oversized batch")
	}
}

func TestValidateBatchSendDefersItemValidation(t *testing.T) {
	err := validateBatchSend(BatchSendRequest{Messages: []SendRequest{
		{To: "+233241234567", From: "DUGBLE", Body: "hello"},
		{To: "0241234567", From: "DUGBLE", Body: "hello"},
	}})
	if err != nil {
		t.Fatalf("validateBatchSend returned error for item-level validation: %v", err)
	}
}

func TestCountSegments(t *testing.T) {
	if got := countSegments("hello"); got != 1 {
		t.Fatalf("countSegments short = %d, want 1", got)
	}
	long := make([]rune, 161)
	for i := range long {
		long[i] = 'a'
	}
	if got := countSegments(string(long)); got != 2 {
		t.Fatalf("countSegments long = %d, want 2", got)
	}

	ucs2 := make([]rune, 71)
	for i := range ucs2 {
		ucs2[i] = '界'
	}
	if got := countSegments(string(ucs2)); got != 2 {
		t.Fatalf("countSegments UCS-2 = %d, want 2", got)
	}

	emoji := make([]rune, 36)
	for i := range emoji {
		emoji[i] = '😀'
	}
	if got := countSegments(string(emoji)); got != 2 {
		t.Fatalf("countSegments emoji = %d, want 2", got)
	}

	extended := make([]rune, 81)
	for i := range extended {
		extended[i] = '^'
	}
	if got := countSegments(string(extended)); got != 2 {
		t.Fatalf("countSegments GSM-7 extended = %d, want 2", got)
	}
}

func TestSMSResponseJSONUsesPublicRepresentation(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	providerID := "arkesel"
	providerMessageID := "provider-secret"
	internalError := "upstream payload that must not be exposed"
	message := Message{
		ID:                 "message-id",
		TeamID:             "team-id",
		To:                 "+233241234567",
		From:               "DUGBLE",
		Body:               "hello",
		Status:             StatusFailed,
		ProviderID:         &providerID,
		ProviderMessageID:  &providerMessageID,
		DestinationCountry: "GH",
		Segments:           1,
		ErrorMessage:       &internalError,
		Metadata:           json.RawMessage(`{"campaign":"welcome"}`),
		Tags:               []Tag{{Name: "category", Value: "welcome_sms"}},
		SubmittedAt:        &now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	payload, err := json.Marshal(message.Response())
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	body := string(payload)
	for _, hidden := range []string{
		"team_id",
		"sender_id",
		"provider_id",
		"provider_message_id",
		"error_message",
		internalError,
	} {
		if strings.Contains(body, hidden) {
			t.Fatalf("SMS response JSON should not expose %s: %s", hidden, body)
		}
	}
	for _, expected := range []string{
		`"object":"sms"`,
		`"message_id":"provider-secret"`,
		`"last_event":"failed"`,
		`"tags":[{"name":"category","value":"welcome_sms"}]`,
		`"metadata":{"campaign":"welcome"}`,
		`"destination":{"country":"GH"}`,
		`"segments":1`,
		`"submitted_at":"2026-07-24T12:00:00Z"`,
		`"updated_at":"2026-07-24T12:00:00Z"`,
		`"failure":{"code":"SMS_FAILED","message":"SMS delivery failed"}`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("SMS response JSON missing %s: %s", expected, body)
		}
	}
}

func TestSMSSendResponseIsCompact(t *testing.T) {
	payload, err := json.Marshal((Message{ID: "message-id", Body: "private"}).SendResponse())
	if err != nil {
		t.Fatalf("marshal send response: %v", err)
	}
	if string(payload) != `{"object":"sms","id":"message-id"}` {
		t.Fatalf("send response = %s", payload)
	}
}

func TestSMSSendResponsesPreserveBatchOrder(t *testing.T) {
	responses := SendResponses([]Message{{ID: "first"}, {ID: "second"}})
	if len(responses) != 2 || responses[0].ID != "first" || responses[1].ID != "second" {
		t.Fatalf("send responses = %#v", responses)
	}
}

func TestBatchSendRequestAcceptsTopLevelArray(t *testing.T) {
	var request BatchSendRequest
	if err := json.Unmarshal([]byte(`[
		{"to":"+233241234567","from":"DUGBLE","body":"first"},
		{"to":"+233201234567","from":"DUGBLE","body":"second"}
	]`), &request); err != nil {
		t.Fatalf("unmarshal batch: %v", err)
	}
	if len(request.Messages) != 2 || request.Messages[1].Body != "second" {
		t.Fatalf("unexpected batch: %#v", request)
	}
}

func TestValidateSendNormalizesTags(t *testing.T) {
	request, err := validateSend(SendRequest{
		To: "+233241234567", From: "DUGBLE", Body: "hello",
		Tags: []Tag{{Name: " category ", Value: " welcome_sms "}},
	})
	if err != nil {
		t.Fatalf("validate send: %v", err)
	}
	if len(request.Tags) != 1 || request.Tags[0].Name != "category" || request.Tags[0].Value != "welcome_sms" {
		t.Fatalf("unexpected tags: %#v", request.Tags)
	}
	request.Tags[0].Value = "not valid"
	if _, err := validateSend(request); err == nil {
		t.Fatal("expected invalid tag to be rejected")
	}
}

func TestValidateSendNormalizesSchedule(t *testing.T) {
	request, err := validateSend(SendRequest{To: "+233241234567", From: "DUGBLE", Body: "hello", ScheduledAt: "in 5 min"})
	if err != nil {
		t.Fatalf("validate scheduled send: %v", err)
	}
	when, err := time.Parse(time.RFC3339Nano, request.ScheduledAt)
	if err != nil || time.Until(when) < 4*time.Minute {
		t.Fatalf("unexpected schedule: %q", request.ScheduledAt)
	}
	if _, err := normalizeSMSSchedule("in 5 min", false); err == nil {
		t.Fatal("expected relative update schedule to be rejected")
	}
}

func TestScheduleRequiresLeadTime(t *testing.T) {
	tooSoon := time.Now().UTC().Add(minimumScheduleLeadTime / 2).Format(time.RFC3339Nano)
	if _, err := normalizeSMSSchedule(tooSoon, false); err == nil {
		t.Fatal("expected schedule inside minimum lead time to be rejected")
	}
	valid := time.Now().UTC().Add(minimumScheduleLeadTime + time.Minute).Format(time.RFC3339Nano)
	if _, err := normalizeSMSSchedule(valid, false); err != nil {
		t.Fatalf("expected schedule outside minimum lead time to pass: %v", err)
	}
}

func TestResponsesMapsEveryMessageToPublicDTO(t *testing.T) {
	responses := Responses([]Message{{ID: "first"}, {ID: "second"}})
	if len(responses) != 2 || responses[0].ID != "first" || responses[1].ID != "second" {
		t.Fatalf("Responses() = %#v", responses)
	}
}

func TestResolveProviderStatusPreventsRegression(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		provider string
		want     string
	}{
		{name: "unknown does not replace submitted", current: StatusSubmitted, provider: StatusUnknown, want: StatusSubmitted},
		{name: "submitted does not replace sent", current: StatusSent, provider: StatusSubmitted, want: StatusSent},
		{name: "submitted does not replace delivered", current: StatusDelivered, provider: StatusSubmitted, want: StatusDelivered},
		{name: "failure does not replace delivered", current: StatusDelivered, provider: StatusFailed, want: StatusDelivered},
		{name: "sent advances submitted", current: StatusSubmitted, provider: StatusSent, want: StatusSent},
		{name: "delivered advances sent", current: StatusSent, provider: StatusDelivered, want: StatusDelivered},
		{name: "rejected closes submitted", current: StatusSubmitted, provider: StatusRejected, want: StatusRejected},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveProviderStatus(test.current, test.provider); got != test.want {
				t.Fatalf("resolveProviderStatus(%q, %q) = %q, want %q", test.current, test.provider, got, test.want)
			}
		})
	}
}
