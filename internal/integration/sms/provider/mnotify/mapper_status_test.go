package mnotify

import (
	"errors"
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

func TestFromInternalFormatsE164Recipient(t *testing.T) {
	t.Parallel()

	request := FromInternal(sms.SendRequest{
		To:      " +233201234567 ",
		From:    " DUGBLE ",
		Message: "hello",
	})

	if len(request.Recipient) != 1 || request.Recipient[0] != "233201234567" {
		t.Fatalf("recipient = %#v, want digits-only international number", request.Recipient)
	}
	if request.Sender != "DUGBLE" {
		t.Fatalf("sender = %q, want DUGBLE", request.Sender)
	}
	if request.IsSchedule || request.ScheduleDate != "" {
		t.Fatalf("provider scheduling should remain disabled: %#v", request)
	}
}

func TestToInternalReturnsDefinitiveAPIErrorForBodyFailure(t *testing.T) {
	t.Parallel()

	_, err := ToInternal(&SendResponse{
		Status:  "error",
		Code:    ResponseCode("1001"),
		Message: "invalid sender",
	})

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T, want *APIError", err)
	}
	if !apiErr.SafeToFallback() {
		t.Fatal("body-level rejection must be classified as definitive")
	}
}

func TestNormalizeStatusPreservesDeliveryMeaning(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"PENDING":       sms.StatusSubmitted,
		"SENT":          sms.StatusSent,
		"DELIVERED":     sms.StatusDelivered,
		"UNDELIV":       sms.StatusUndelivered,
		"REJECTED":      sms.StatusRejected,
		"FAILED":        sms.StatusFailed,
		"EXPIRED":       sms.StatusExpired,
		"something-new": sms.StatusUnknown,
	}
	for input, want := range tests {
		if got := NormalizeStatus(input); got != want {
			t.Errorf("NormalizeStatus(%q) = %q, want %q", input, got, want)
		}
	}
}
