package webhook

import "errors"

var (
	ErrClaimLost              = errors.New("webhook delivery claim was lost")
	ErrQueueNotConfigured     = errors.New("webhook delivery queue is not configured")
	ErrClientNotConfigured    = errors.New("webhook HTTP client is not configured")
	ErrProcessorNotConfigured = errors.New("webhook delivery processor is not configured")
	ErrInvalidDelivery        = errors.New("webhook delivery is invalid")
)
