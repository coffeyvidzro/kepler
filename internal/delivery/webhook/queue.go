package webhook

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ClaimQueue leases pending deliveries to workers.
type ClaimQueue interface {
	Claim(context.Context, string, int32, time.Time) ([]ClaimedDelivery, error)
}

// ResultQueue records the outcome of a leased delivery.
type ResultQueue interface {
	MarkSucceeded(context.Context, uuid.UUID, string, int32, *string) error
	ScheduleRetry(context.Context, uuid.UUID, string, time.Time, *int32, *string, string) error
	MarkFailed(context.Context, uuid.UUID, string, *int32, *string, string) error
	ReleaseClaim(context.Context, uuid.UUID, string) error
}

// Queue is the complete persistence contract used by webhook delivery workers.
type Queue interface {
	ClaimQueue
	ResultQueue
}
