package tenantprovision

import "errors"

var (
	ErrQueueNotConfigured     = errors.New("email tenant provisioning queue is not configured")
	ErrConsumerNotConfigured  = errors.New("email tenant provisioning consumer is not fully configured")
	ErrProcessorNotConfigured = errors.New("email tenant provisioning processor is not configured")
	ErrTransactionRequired    = errors.New("email tenant provisioning requires a PostgreSQL transaction")
)

func errorsNewTransactionRequired() error {
	return ErrTransactionRequired
}
