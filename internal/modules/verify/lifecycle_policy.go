package verify

import (
	"time"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func terminalCheckResponse(current Verification) CheckResponse {
	return CheckResponse{
		ID:      current.ID,
		Status:  current.Status,
		Valid:   false,
		Expired: current.Status == StatusExpired,
	}
}

func validateResendVerification(current Verification, configured VerificationService, now time.Time) error {
	if current.Status != StatusPending {
		return apperrors.NewConflict("Only pending verifications can be resent")
	}
	if current.ResendCount >= configured.MaxResends {
		return apperrors.TooManyRequests("Verification resend limit reached")
	}
	if !current.ExpiresAt.After(now) {
		return apperrors.NewConflict("Expired verifications cannot be resent")
	}
	return nil
}

func validateResendChallenge(createdAt, expiresAt time.Time, cooldownSeconds int32, now time.Time) error {
	if !expiresAt.After(now) {
		return apperrors.NewConflict("Expired verifications cannot be resent")
	}
	nextAllowed := createdAt.Add(time.Duration(cooldownSeconds) * time.Second)
	if now.Before(nextAllowed) {
		return apperrors.TooManyRequests("Verification resend cooldown is still active")
	}
	return nil
}
