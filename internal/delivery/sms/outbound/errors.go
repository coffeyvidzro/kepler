package smsdelivery

import "errors"

var (
	ErrQueueNotConfigured     = errors.New("SMS delivery outbox is not configured")
	ErrConsumerNotConfigured  = errors.New("SMS delivery consumer is not configured")
	ErrProcessorNotConfigured = errors.New("SMS delivery processor is not configured")
)
