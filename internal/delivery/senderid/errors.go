package senderidreconciliation

import "errors"

const (
	providerStatusSubmissionFailed  = "submission_failed"
	providerStatusSubmissionUnknown = "submission_unknown"
)

var (
	ErrConsumerNotConfigured = errors.New("Sender ID reconciliation consumer is not configured")
	ErrInvalidConfig         = errors.New("invalid Sender ID reconciliation configuration")
	ErrWorkerIDRequired      = errors.New("Sender ID reconciliation worker ID is required")
)

type safeFallbackError interface {
	error
	SafeToFallback() bool
}

func definitiveProviderError(err error) bool {
	var definitive safeFallbackError
	return errors.As(err, &definitive) && definitive.SafeToFallback()
}
