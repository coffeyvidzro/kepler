package audit

import (
	"time"

	"github.com/google/uuid"
)

const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

type Event struct {
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]any
	Outcome      string
}

type Entry struct {
	ID             uuid.UUID
	TeamID         uuid.UUID
	ActorType      string
	ActorUserID    uuid.UUID
	ActorSessionID string
	ActorTokenID   uuid.UUID
	Action         string
	ResourceType   string
	ResourceID     string
	Outcome        string
	Metadata       map[string]any
	Request        RequestMetadata
	CreatedAt      time.Time
}

func newEntry(event Event) Entry {
	return Entry{
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Outcome:      normalizedOutcome(event.Outcome),
		Metadata:     normalizedMetadata(event.Metadata),
	}
}

func normalizedOutcome(outcome string) string {
	if outcome == "" {
		return OutcomeSuccess
	}
	return outcome
}

func normalizedMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	return metadata
}
