package outbox

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID            uuid.UUID
	Subject       string
	AggregateType string
	AggregateID   uuid.UUID
	Payload       json.RawMessage
	Headers       map[string]string
	AvailableAt   time.Time
	Attempts      int
	CreatedAt     time.Time
}
