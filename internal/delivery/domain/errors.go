package domainreconciliation

import "errors"

var (
	ErrConsumerNotConfigured = errors.New("sender domain reconciliation consumer is not configured")
	ErrInvalidConfig         = errors.New("sender domain reconciliation configuration is invalid")
	ErrWorkerIDRequired      = errors.New("sender domain reconciliation worker ID is required")
)
