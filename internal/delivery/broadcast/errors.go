package broadcastexecution

import "errors"

var (
	// ErrConsumerNotConfigured indicates missing polling dependencies.
	ErrConsumerNotConfigured = errors.New("broadcast execution consumer is not configured")
	// ErrProcessorNotConfigured indicates missing execution persistence.
	ErrProcessorNotConfigured = errors.New("broadcast execution processor is not configured")
)
