package senderidreconciliation

import "errors"

const (
	providerStatusSubmissionFailed  = "submission_failed"
	providerStatusSubmissionUnknown = "submission_unknown"
)

var (
	ErrConsumerNotConfigured = errors.New("sender ID reconciliation consumer is not configured")
	ErrInvalidConfig         = errors.New("invalid Sender ID reconciliation configuration")
	ErrRegistrationClaimLost = errors.New("sender ID registration claim was lost")
	ErrWorkerIDRequired      = errors.New("sender ID reconciliation worker ID is required")
)

type safeFallbackError interface {
	error
	SafeToFallback() bool
}

func definitiveProviderError(err error) bool {
	var definitive safeFallbackError
	return errors.As(err, &definitive) && definitive.SafeToFallback()
}
