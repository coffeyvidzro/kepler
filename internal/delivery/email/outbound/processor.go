package emaildelivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
)

type deliveryRepository interface {
	Claim(context.Context, uuid.UUID, uuid.UUID) (DeliveryMessage, error)
	MarkRequestStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	MarkSubmitted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, platformemail.Result) error
	MarkRetryable(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error
	MarkSubmissionUnknown(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, error) error
	MarkFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, error) error
	MarkExhausted(context.Context, uuid.UUID, uuid.UUID, error) error
}

type Processor struct {
	repository deliveryRepository
	sender     platformemail.Sender
}

type Handler = Processor

func NewProcessor(repository deliveryRepository, sender platformemail.Sender) *Processor {
	return &Processor{repository: repository, sender: sender}
}

func NewHandler(repository deliveryRepository, sender platformemail.Sender) *Processor {
	return NewProcessor(repository, sender)
}

func (processor *Processor) Handle(ctx context.Context, command DeliverCommand) error {
	if processor == nil || processor.repository == nil {
		return errors.New("email delivery repository is not configured")
	}
	if processor.sender == nil {
		return errors.New("email sender is not configured")
	}
	message, err := processor.repository.Claim(ctx, command.MessageID, command.TeamID)
	if errors.Is(err, ErrMessageNotDeliverable) || errors.Is(err, ErrSenderDomainUnavailable) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := processor.repository.MarkRequestStarted(ctx, command.MessageID, command.TeamID, message.AttemptID); err != nil {
		return err
	}
	route, applicationHeaders := platformemail.ExtractDeliveryRoute(message.Headers)
	result, err := processor.sender.Send(ctx, platformemail.Message{
		MessageID:        message.ID.String(),
		AttemptID:        message.AttemptID.String(),
		Provider:         message.Provider,
		Region:           message.Region,
		Stream:           route.Stream,
		ConfigurationSet: route.ConfigurationSet,
		SESTenantName:    route.SESTenantName,
		From:             platformemail.Address{Email: message.FromEmail, Name: message.FromName},
		ReplyTo:          message.ReplyTo,
		To:               message.To,
		CC:               message.CC,
		BCC:              message.BCC,
		Subject:          message.Subject,
		HTML:             message.HTML,
		Text:             message.Text,
		Headers:          applicationHeaders,
		Attachments:      message.Attachments,
	})
	if err != nil {
		if platformemail.IsSubmissionUnknown(err) {
			if recordErr := processor.repository.MarkSubmissionUnknown(
				ctx, command.MessageID, command.TeamID, message.AttemptID,
				platformemail.FailureCode(err), err,
			); recordErr != nil {
				return errors.Join(err, recordErr)
			}
			return nil
		}
		if platformemail.IsRetryable(err) {
			if recordErr := processor.repository.MarkRetryable(ctx, command.MessageID, command.TeamID, message.AttemptID, err); recordErr != nil {
				return errors.Join(err, recordErr)
			}
			return fmt.Errorf("send email: %w", err)
		}
		if recordErr := processor.repository.MarkFailed(
			ctx, command.MessageID, command.TeamID, message.AttemptID,
			platformemail.FailureCode(err), err,
		); recordErr != nil {
			return errors.Join(err, recordErr)
		}
		return nil
	}

	if err := processor.repository.MarkSubmitted(ctx, command.MessageID, command.TeamID, message.AttemptID, result); err != nil {
		unknownErr := fmt.Errorf("persist provider submission result: %w", err)
		if recordErr := processor.repository.MarkSubmissionUnknown(
			ctx, command.MessageID, command.TeamID, message.AttemptID,
			"submission_persistence_failed", unknownErr,
		); recordErr != nil {
			return errors.Join(unknownErr, recordErr)
		}
		return nil
	}
	return nil
}

func (processor *Processor) HandleExhausted(ctx context.Context, command DeliverCommand, cause error) error {
	if processor == nil || processor.repository == nil {
		return errors.New("email delivery repository is not configured")
	}
	return processor.repository.MarkExhausted(ctx, command.MessageID, command.TeamID, cause)
}
