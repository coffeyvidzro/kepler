package server

import (
	"errors"

	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

type Dependencies struct {
	WebhookEmitter *platformwebhook.Emitter
}

func (dependencies Dependencies) validate() error {
	if dependencies.WebhookEmitter == nil {
		return errors.New("webhook emitter is required")
	}
	return nil
}
