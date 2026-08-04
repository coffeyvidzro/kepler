package verify

import (
	"testing"
	"time"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func TestTerminalCheckResponseNeverAuthenticatesNewSubmission(t *testing.T) {
	t.Parallel()

	for _, status := range []string{
		StatusApproved,
		StatusExpired,
		StatusCanceled,
		StatusMaxAttemptsReached,
		StatusDeliveryFailed,
	} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			response := terminalCheckResponse(Verification{ID: "verification-id", Status: status})
			if response.Valid {
				t.Fatalf("terminalCheckResponse(%q).Valid = true", status)
			}
			if response.Expired != (status == StatusExpired) {
				t.Fatalf("terminalCheckResponse(%q).Expired = %v", status, response.Expired)
			}
		})
	}
}

func TestValidateResendVerification(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 4, 17, 0, 0, 0, time.UTC)
	configured := VerificationService{MaxResends: 2}

	tests := []struct {
		name         string
		verification Verification
		code         string
	}{
		{name: "terminal", verification: Verification{Status: StatusApproved, ExpiresAt: now.Add(time.Minute)}, code: "CONFLICT"},
		{name: "quota", verification: Verification{Status: StatusPending, ResendCount: 2, ExpiresAt: now.Add(time.Minute)}, code: "TOO_MANY_REQUESTS"},
		{name: "expired", verification: Verification{Status: StatusPending, ExpiresAt: now}, code: "CONFLICT"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateResendVerification(test.verification, configured, now)
			var appError *apperrors.AppError
			if err == nil || !asAppError(err, &appError) || appError.Code != test.code {
				t.Fatalf("validateResendVerification() error = %v, want %s", err, test.code)
			}
		})
	}
}

func TestValidateResendChallenge(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 4, 17, 0, 0, 0, time.UTC)
	if err := validateResendChallenge(now.Add(-time.Minute), now.Add(time.Minute), 30, now); err != nil {
		t.Fatalf("validateResendChallenge() error = %v", err)
	}
	if err := validateResendChallenge(now.Add(-10*time.Second), now.Add(time.Minute), 30, now); err == nil {
		t.Fatal("validateResendChallenge() accepted active cooldown")
	}
	if err := validateResendChallenge(now.Add(-time.Minute), now, 0, now); err == nil {
		t.Fatal("validateResendChallenge() accepted expired challenge")
	}
}

func asAppError(err error, target **apperrors.AppError) bool {
	appError, ok := err.(*apperrors.AppError)
	if ok {
		*target = appError
	}
	return ok
}
