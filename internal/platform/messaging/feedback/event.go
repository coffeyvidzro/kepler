package feedback

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
)

// Event is a normalized provider delivery-status event.
type Event struct {
	Provider          string
	ProviderEventID   string
	ProviderMessageID string
	Channel           messaging.Channel
	Status            delivery.AttemptStatus
	ProviderStatus    string
	ErrorCode         string
	ErrorMessage      string
	OccurredAt        time.Time
	ReceivedAt        time.Time
	Metadata          json.RawMessage
}

// DedupeKey is stable across retries of the same provider event.
func (event Event) DedupeKey() string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(event.Provider)),
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
	if !event.Channel.Valid() {
		return errors.New("feedback channel is invalid")
	}
	if !event.Status.Valid() {
		return errors.New("feedback delivery status is invalid")
	}
	if event.Status == delivery.StatusClaimed || event.Status == delivery.StatusRequestStarted {
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
