package webhook

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ClaimedDelivery is a delivery attempt leased to one worker.
type ClaimedDelivery struct {
	ID            uuid.UUID
	EventID       uuid.UUID
	EndpointID    uuid.UUID
	AttemptCount  int32
	TeamID        uuid.UUID
	EventType     string
	Payload       json.RawMessage
	OccurredAt    time.Time
	URL           string
	SigningSecret []byte
}

// HTTPResponse is the bounded response captured from a webhook endpoint.
type HTTPResponse struct {
	StatusCode int
	Body       string
	Header     http.Header
}
