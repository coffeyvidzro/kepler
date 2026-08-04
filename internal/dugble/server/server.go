package server

import (
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

type Runtime struct {
	Events *platformevent.Emitter
}

func New(dependencies Dependencies) (*Runtime, error) {
	if err := dependencies.validate(); err != nil {
		return nil, err
	}
	return &Runtime{
		Events: platformevent.NewEmitter(
			platformwebhook.NewEventSink(dependencies.WebhookEmitter),
		),
	}, nil
}
