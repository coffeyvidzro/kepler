package systememail

import (
	"context"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type Processor struct {
	sender platformemail.Sender
}

func NewProcessor(sender platformemail.Sender) *Processor {
	return &Processor{sender: sender}
}

func (processor *Processor) Handle(ctx context.Context, command DeliverCommand) error {
	if processor == nil || processor.sender == nil {
		return ErrProcessorNotConfigured
	}
	if err := ValidateCommand(command); err != nil {
		return err
	}
	_, err := processor.sender.Send(ctx, command.Message)
	return err
}
