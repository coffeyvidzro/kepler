package contact

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type eventEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformevent.Envelope) (platformevent.Result, error)
}

func emitContactEvent(ctx context.Context, tx pgx.Tx, emitter eventEmitter, eventType platformevent.Type, current Contact, previous *Contact) error {
	if emitter == nil {
		return nil
	}
	teamID, err := uuid.Parse(current.TeamID)
	if err != nil {
		return fmt.Errorf("parse contact team id: %w", err)
	}
	objectID, err := uuid.Parse(current.ID)
	if err != nil {
		return fmt.Errorf("parse contact id: %w", err)
	}
	payload := map[string]any{"contact": current}
	if previous != nil {
		payload["changed_fields"] = changedContactFields(*previous, current)
		if previous.Email != current.Email {
			payload["previous_email"] = previous.Email
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode contact event: %w", err)
	}
	_, err = emitter.EmitTx(ctx, tx, platformevent.Envelope{
		Type:       eventType,
		TeamID:     teamID,
		ObjectType: "contact",
		ObjectID:   &objectID,
		Data:       data,
	})
	return err
}

func changedContactFields(previous, current Contact) []string {
	fields := make([]string, 0)
	if previous.Email != current.Email {
		fields = append(fields, "email")
	}
	if !reflect.DeepEqual(previous.FirstName, current.FirstName) {
		fields = append(fields, "first_name")
	}
	if !reflect.DeepEqual(previous.LastName, current.LastName) {
		fields = append(fields, "last_name")
	}
	if previous.Unsubscribed != current.Unsubscribed {
		fields = append(fields, "unsubscribed")
	}
	keys := map[string]struct{}{}
	for key := range previous.Properties {
		keys[key] = struct{}{}
	}
	for key := range current.Properties {
		keys[key] = struct{}{}
	}
	for key := range keys {
		if !reflect.DeepEqual(previous.Properties[key], current.Properties[key]) {
			fields = append(fields, "properties."+key)
		}
	}
	sort.Strings(fields)
	return fields
}
