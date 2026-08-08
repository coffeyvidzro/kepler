package feedback

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/delivery/attempt"
)

// Event is a normalized provider delivery-status or engagement event.
type Event struct {
	AttemptID         uuid.UUID             `json:"attempt_id,omitempty"`
	Provider          string                `json:"provider"`
	ProviderEventID   string                `json:"provider_event_id"`
	ProviderMessageID string                `json:"provider_message_id"`
	EventType         string                `json:"event_type"`
	Channel           attempt.Channel       `json:"channel"`
	Status            attempt.AttemptStatus `json:"status"`
	ProviderStatus    string                `json:"provider_status,omitempty"`
	ErrorCode         string                `json:"error_code,omitempty"`
	ErrorMessage      string                `json:"error_message,omitempty"`
	OccurredAt        time.Time             `json:"occurred_at"`
	ReceivedAt        time.Time             `json:"received_at"`
	Metadata          json.RawMessage       `json:"metadata,omitempty"`
}

// DedupeKey is stable across retries of the same provider event.
func (event Event) DedupeKey() string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(event.Provider)),
		string(event.Channel),
		strings.TrimSpace(event.ProviderEventID),
	}, ":")
}

func (event Event) Validate() error {
	if strings.TrimSpace(event.Provider) == "" {
		return errors.New("feedback provider is required")
	}
	if strings.TrimSpace(event.ProviderEventID) == "" {
		return errors.New("feedback provider event ID is required")
	}
	if strings.TrimSpace(event.ProviderMessageID) == "" {
		return errors.New("feedback provider message ID is required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return errors.New("feedback event type is required")
	}
	if !event.Channel.Valid() {
		return errors.New("feedback channel is invalid")
	}
	if !event.Status.Valid() {
		return errors.New("feedback delivery status is invalid")
	}
	if event.Status == attempt.StatusClaimed || event.Status == attempt.StatusRequestStarted {
		return fmt.Errorf("feedback cannot report internal delivery status %q", event.Status)
	}
	if event.OccurredAt.IsZero() || event.ReceivedAt.IsZero() {
		return errors.New("feedback timestamps are required")
	}
	if event.ReceivedAt.Before(event.OccurredAt) {
		return errors.New("feedback cannot be received before it occurred")
	}
	if !validJSONObject(event.Metadata) {
		return errors.New("feedback metadata must be a JSON object")
	}
	return nil
}

func validJSONObject(value json.RawMessage) bool {
	if len(bytes.TrimSpace(value)) == 0 {
		return true
	}
	if !json.Valid(value) {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil
}
