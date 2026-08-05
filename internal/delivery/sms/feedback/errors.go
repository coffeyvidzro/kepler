package feedback

import "errors"

var (
	ErrRepositoryNotConfigured = errors.New("SMS feedback repository is not configured")
	ErrProcessorNotConfigured  = errors.New("SMS feedback processor is not configured")
	ErrReconcilerNotConfigured = errors.New("SMS feedback reconciler is not configured")
	ErrConsumerNotConfigured   = errors.New("SMS feedback consumer is not configured")
	ErrProviderRequired        = errors.New("SMS feedback provider ID is required")
	ErrProviderMessageRequired = errors.New("SMS feedback provider message ID is required")
	ErrUnsupportedStatus       = errors.New("SMS feedback status is not supported")
)
