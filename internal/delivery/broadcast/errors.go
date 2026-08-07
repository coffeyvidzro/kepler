package broadcastexecution

import "errors"

var (
	ErrConsumerNotConfigured  = errors.New("broadcast execution consumer is not configured")
	ErrProcessorNotConfigured = errors.New("broadcast execution processor is not configured")
)
