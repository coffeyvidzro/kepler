package smsdelivery

import (
	"context"

	"github.com/google/uuid"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type messageRepository interface {
	MarkProcessing(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error)
	MarkDeliveryUnknown(context.Context, uuid.UUID, uuid.UUID, string) (smsmodule.Message, error)
	MarkFailed(context.Context, uuid.UUID, uuid.UUID, string) (smsmodule.Message, error)
	ResolveDeliveryRoutes(context.Context, uuid.UUID, uuid.UUID) ([]platformrouting.Route, error)
	CreateDeliveryAttempt(context.Context, uuid.UUID, uuid.UUID, platformrouting.Route) (uuid.UUID, error)
	MarkDeliveryAttemptStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	MarkDeliveryAttemptRetryable(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error
	MarkDeliveryAttemptUnknown(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error
	MarkDeliveryAttemptFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error
	MarkDeliveryAttemptSubmitted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *smsapi.SendResponse) error
	FinalizeInFlightDelivery(context.Context, uuid.UUID, uuid.UUID, error) error
}

type providerSender interface {
	SendWithProvider(context.Context, string, smsapi.SendRequest) (*smsapi.SendResponse, error)
	ShouldFallback(context.Context, string, error) bool
	ProviderIDs() []string
}
