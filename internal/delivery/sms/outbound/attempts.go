package smsdelivery

import (
	"time"

	"github.com/google/uuid"
)

type AttemptStatus string

const (
	AttemptQueued            AttemptStatus = "queued"
	AttemptProcessing        AttemptStatus = "processing"
	AttemptSubmitted         AttemptStatus = "submitted"
	AttemptFailed            AttemptStatus = "failed"
	AttemptSubmissionUnknown AttemptStatus = "submission_unknown"
)

type Attempt struct {
	MessageID         uuid.UUID
	TeamID            uuid.UUID
	ProviderID        string
	ProviderMessageID string
	Status            AttemptStatus
	StartedAt         time.Time
	CompletedAt       *time.Time
	ErrorMessage      string
}
