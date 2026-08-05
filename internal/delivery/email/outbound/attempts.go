package emaildelivery

import (
	"time"

	"github.com/google/uuid"
)

type AttemptStatus string

const (
	AttemptClaimed           AttemptStatus = "claimed"
	AttemptRequestStarted    AttemptStatus = "request_started"
	AttemptSubmitted         AttemptStatus = "submitted"
	AttemptRetryableFailure  AttemptStatus = "retryable_failure"
	AttemptSubmissionUnknown AttemptStatus = "submission_unknown"
	AttemptFailed            AttemptStatus = "failed"
)

type Attempt struct {
	ID                uuid.UUID
	MessageID         uuid.UUID
	TeamID            uuid.UUID
	Number            int
	Status            AttemptStatus
	Provider          string
	ProviderMessageID string
	StartedAt         *time.Time
	CompletedAt       *time.Time
	ErrorCode         string
	ErrorMessage      string
}
