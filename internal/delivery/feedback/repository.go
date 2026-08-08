package feedback

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/delivery/attempt"
)

var (
	ErrAttemptNotFound  = errors.New("delivery attempt was not found for provider feedback")
	ErrConcurrentUpdate = errors.New("delivery attempt changed while applying provider feedback")
)

// Lookup identifies the attempt targeted by a normalized provider event.
type Lookup struct {
	AttemptID         uuid.UUID
	Provider          string
	ProviderMessageID string
	Channel           attempt.Channel
}

// AttemptUpdate is the conditional state change persisted with an event.
type AttemptUpdate struct {
	AttemptID      uuid.UUID
	ExpectedStatus attempt.AttemptStatus
	Status         *attempt.AttemptStatus
	ProviderStatus string
	ErrorCode      string
	ErrorMessage   string
	OccurredAt     time.Time
	TerminalAt     *time.Time
	ReconciledAt   time.Time
}

// ApplyResult reports whether an event was recorded, deduplicated, or changed state.
type ApplyResult struct {
	Applied      bool
	Duplicate    bool
	Transitioned bool
}

// Repository provides the atomic persistence boundary for feedback processing.
// ApplyEvent must deduplicate by Event.DedupeKey and conditionally update the
// attempt from ExpectedStatus in the same transaction.
type Repository interface {
	FindAttempt(context.Context, Lookup) (attempt.Attempt, error)
	ApplyEvent(context.Context, Event, AttemptUpdate) (ApplyResult, error)
}
