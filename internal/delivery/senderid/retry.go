package senderidreconciliation

import (
	"context"
	"errors"
	"time"
)

func (consumer *Consumer) recordFailure(
	ctx context.Context,
	claim RegistrationClaim,
	providerStatus string,
	cause error,
) error {
	nextCheckAt := consumer.now().Add(consumer.retryDelay(claim.Attempt))
	recordErr := consumer.repository.RecordProviderFailure(
		ctx,
		claim.ID,
		consumer.workerID,
		providerStatus,
		cause,
		nextCheckAt,
	)
	return errors.Join(cause, recordErr)
}

func (consumer *Consumer) retryDelay(attempt int32) time.Duration {
	delay := consumer.config.RetryBaseInterval
	for current := int32(1); current < attempt && delay < consumer.config.MaxRetryInterval; current++ {
		if delay > consumer.config.MaxRetryInterval/2 {
			return consumer.config.MaxRetryInterval
		}
		delay *= 2
	}
	if delay > consumer.config.MaxRetryInterval {
		return consumer.config.MaxRetryInterval
	}
	return delay
}
