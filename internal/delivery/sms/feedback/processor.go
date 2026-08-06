package feedback

import (
	"context"

	platformfeedback "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/feedback"
)

type eventRepository interface {
	Apply(context.Context, platformfeedback.Event) (platformfeedback.Result, error)
}

type Processor struct {
	repository eventRepository
}

func NewProcessor(repository eventRepository) *Processor {
	return &Processor{repository: repository}
}

func (processor *Processor) Handle(
	ctx context.Context,
	event platformfeedback.Event,
) (platformfeedback.Result, error) {
	if processor == nil || processor.repository == nil {
		return platformfeedback.Result{}, ErrProcessorNotConfigured
	}
	if err := event.Validate(); err != nil {
		return platformfeedback.Result{}, err
	}
	return processor.repository.Apply(ctx, event)
}
