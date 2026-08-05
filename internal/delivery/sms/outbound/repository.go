package smsdelivery

import (
	"context"

	"github.com/google/uuid"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
)

type messageRepository interface {
	MarkProcessing(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error)
	MarkDeliveryUnknown(context.Context, uuid.UUID, uuid.UUID, string) (smsmodule.Message, error)
	MarkFailed(context.Context, uuid.UUID, uuid.UUID, string) (smsmodule.Message, error)
	MarkSubmitted(context.Context, uuid.UUID, uuid.UUID, string, string, string) (smsmodule.Message, error)
}
