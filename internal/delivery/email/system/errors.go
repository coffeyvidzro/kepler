package systememail

import "errors"

var (
	ErrQueueNotConfigured     = errors.New("system email outbox is not configured")
	ErrConsumerNotConfigured  = errors.New("system email consumer is not fully configured")
	ErrProcessorNotConfigured = errors.New("system email processor is not configured")
)
