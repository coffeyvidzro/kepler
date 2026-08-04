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

type Handler struct {
	repository deliveryRepository
	sender     platformemail.Sender
}

func NewHandler(repository deliveryRepository, sender platformemail.Sender) *Handler {
	return &Handler{repository: repository, sender: sender}
}

func (h *Handler) Handle(ctx context.Context, command DeliverCommand) error {
	if h == nil || h.repository == nil {
		return errors.New("email delivery repository is not configured")
	}
	if h.sender == nil {
		return errors.New("email sender is not configured")
	}
	message, err := h.repository.Claim(ctx, command.MessageID, command.TeamID)
	if errors.Is(err, ErrMessageNotDeliverable) || errors.Is(err, ErrSenderDomainUnavailable) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := h.repository.MarkRequestStarted(ctx, command.MessageID, command.TeamID, message.AttemptID); err != nil {
		return err
	}
	route, applicationHeaders := platformemail.ExtractDeliveryRoute(message.Headers)
	result, err := h.sender.Send(ctx, platformemail.Message{
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
			if recordErr := h.repository.MarkSubmissionUnknown(
				ctx, command.MessageID, command.TeamID, message.AttemptID,
				platformemail.FailureCode(err), err,
			); recordErr != nil {
				return errors.Join(err, recordErr)
			}
			return nil
		}
		if platformemail.IsRetryable(err) {
			if recordErr := h.repository.MarkRetryable(ctx, command.MessageID, command.TeamID, message.AttemptID, err); recordErr != nil {
				return errors.Join(err, recordErr)
			}
			return fmt.Errorf("send email: %w", err)
		}
		if recordErr := h.repository.MarkFailed(
			ctx, command.MessageID, command.TeamID, message.AttemptID,
			platformemail.FailureCode(err), err,
		); recordErr != nil {
			return errors.Join(err, recordErr)
		}
		return nil
	}

	if err := h.repository.MarkSubmitted(ctx, command.MessageID, command.TeamID, message.AttemptID, result); err != nil {
		unknownErr := fmt.Errorf("persist provider submission result: %w", err)
		if recordErr := h.repository.MarkSubmissionUnknown(
			ctx, command.MessageID, command.TeamID, message.AttemptID,
			"submission_persistence_failed", unknownErr,
		); recordErr != nil {
			return errors.Join(unknownErr, recordErr)
		}
		return nil
	}
	return nil
}

func (h *Handler) HandleExhausted(ctx context.Context, command DeliverCommand, cause error) error {
	if h == nil || h.repository == nil {
		return errors.New("email delivery repository is not configured")
	}
	return h.repository.MarkExhausted(ctx, command.MessageID, command.TeamID, cause)
}
