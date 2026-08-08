package feedback

import (
	"context"
	feedback "github.com/coffeyvidzro/dugble/server/internal/delivery/feedback"
)

type eventRepository interface {
	Apply(context.Context, feedback.Event) (feedback.Result, error)
}

type Processor struct {
	repository eventRepository
}

func NewProcessor(repository eventRepository) *Processor {
	return &Processor{repository: repository}
}

func (processor *Processor) Handle(
	ctx context.Context,
	event feedback.Event,
) (feedback.Result, error) {
	if processor == nil || processor.repository == nil {
		return feedback.Result{}, ErrProcessorNotConfigured
	}
	if err := event.Validate(); err != nil {
		return feedback.Result{}, err
	}
	return processor.repository.Apply(ctx, event)
}
