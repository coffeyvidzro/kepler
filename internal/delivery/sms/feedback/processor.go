package feedback

import "context"

type eventRepository interface {
	Apply(context.Context, Event) error
}

type Processor struct {
	repository eventRepository
}

func NewProcessor(repository eventRepository) *Processor {
	return &Processor{repository: repository}
}

func (processor *Processor) Handle(ctx context.Context, event Event) error {
	if processor == nil || processor.repository == nil {
		return ErrProcessorNotConfigured
	}
	event = event.Normalize()
	if err := event.Validate(); err != nil {
		return err
	}
	return processor.repository.Apply(ctx, event)
}
