package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Envelope struct {
	ID         uuid.UUID       `json:"id"`
	Type       Type            `json:"type"`
	Version    Version         `json:"version"`
	TeamID     uuid.UUID       `json:"team_id"`
	ObjectType string          `json:"object_type"`
	ObjectID   *uuid.UUID      `json:"object_id,omitempty"`
	Data       json.RawMessage `json:"data"`
	OccurredAt time.Time       `json:"occurred_at"`
}

func (envelope Envelope) Normalize(now func() time.Time) (Envelope, error) {
	if now == nil {
		now = time.Now
	}
	if envelope.ID == uuid.Nil {
		envelope.ID = uuid.New()
	}
	if envelope.Version == "" {
		envelope.Version = CurrentVersion
	}
	envelope.Type = Type(strings.TrimSpace(string(envelope.Type)))
	envelope.ObjectType = strings.TrimSpace(envelope.ObjectType)
	if envelope.OccurredAt.IsZero() {
		envelope.OccurredAt = now().UTC()
	} else {
		envelope.OccurredAt = envelope.OccurredAt.UTC()
	}
	if err := envelope.Validate(); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func (envelope Envelope) Validate() error {
	if envelope.ID == uuid.Nil {
		return errors.New("event id is required")
	}
	definition, ok := Lookup(envelope.Type)
	if !ok {
		return fmt.Errorf("unknown event type %q", envelope.Type)
	}
	if !envelope.Version.Valid() {
		return fmt.Errorf("event version %q is not supported", envelope.Version)
	}
	if envelope.TeamID == uuid.Nil {
		return errors.New("event team id is required")
	}
	if envelope.ObjectType == "" {
		return errors.New("event object type is required")
	}
	if envelope.ObjectType != definition.ObjectType {
		return fmt.Errorf("event object type %q does not match %q", envelope.ObjectType, definition.ObjectType)
	}
	if definition.ObjectIDRequired && (envelope.ObjectID == nil || *envelope.ObjectID == uuid.Nil) {
		return errors.New("event object id is required")
	}
	if envelope.ObjectID != nil && *envelope.ObjectID == uuid.Nil {
		return errors.New("event object id must not be nil")
	}
	if envelope.OccurredAt.IsZero() {
		return errors.New("event occurrence time is required")
	}
	data := bytes.TrimSpace(envelope.Data)
	if !json.Valid(data) || !bytes.HasPrefix(data, []byte("{")) {
		return errors.New("event data must be a JSON object")
	}
	return nil
}
