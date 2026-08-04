package email

import (
	"errors"
	"strings"
)

var (
	ErrSandboxTeamEmailNotVerified = errors.New("sandbox team email is not verified")
	ErrSandboxRecipientRestricted  = errors.New("sandbox recipient is restricted")
)

// ValidateSandboxRecipient permits the shared onboarding identity only when
// there is exactly one direct recipient, no copied recipients, and the direct
// recipient matches the team's verified email address.
func ValidateSandboxRecipient(teamEmail, recipientEmail string, toCount, ccCount, bccCount int) error {
	if toCount != 1 || ccCount != 0 || bccCount != 0 {
		return ErrSandboxRecipientRestricted
	}
	if !strings.EqualFold(strings.TrimSpace(recipientEmail), strings.TrimSpace(teamEmail)) {
		return ErrSandboxRecipientRestricted
	}
	return nil
}
