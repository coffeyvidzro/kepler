package systememail

import (
	"errors"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/awsses"
)

const (
	DeliverSubject      = "dugble.job.email.system.v1"
	DeliverConsumerName = "dugble-system-email-delivery-v1"
	deliveryNamespace   = "https://dugble.com/events/email/system/"
)

type DeliverCommand struct {
	EventID       uuid.UUID             `json:"event_id"`
	Message       platformemail.Message `json:"message"`
	SchemaVersion int                   `json:"schema_version"`
}

func ValidateCommand(command DeliverCommand) error {
	if command.EventID == uuid.Nil {
		return errors.New("system email event ID is required")
	}
	if command.SchemaVersion != 1 {
		return errors.New("unsupported system email command schema version")
	}
	return nil
}
