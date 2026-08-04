package webhook

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Command is the stable payload delivered to customer webhook endpoints.
type Command struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Data       json.RawMessage `json:"data"`
}

func NewCommand(delivery ClaimedDelivery) (Command, error) {
	if delivery.EventID == uuid.Nil || strings.TrimSpace(delivery.EventType) == "" {
		return Command{}, ErrInvalidDelivery
	}
	if !json.Valid(delivery.Payload) {
		return Command{}, errors.New("webhook event payload is not valid JSON")
	}
	occurredAt := delivery.OccurredAt.UTC()
	if occurredAt.IsZero() {
		return Command{}, errors.New("webhook event occurrence time is required")
	}
	return Command{
		ID:         delivery.EventID.String(),
		Type:       strings.TrimSpace(delivery.EventType),
		OccurredAt: occurredAt,
		Data:       append(json.RawMessage(nil), delivery.Payload...),
	}, nil
}

func (command Command) Encode() ([]byte, error) {
	return json.Marshal(command)
}
