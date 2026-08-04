package smsdelivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	smsapi "github.com/coffeyvidzro/dugble/server/internal/integration/sms"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
)

const defaultStaleProcessingAfter = 15 * time.Minute

type messageRepository interface {
	MarkProcessing(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (smsmodule.Message, error)
	Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (smsmodule.Message, error)
	MarkDeliveryUnknown(ctx context.Context, id uuid.UUID, teamID uuid.UUID, message string) (smsmodule.Message, error)
	MarkFailed(ctx context.Context, id uuid.UUID, teamID uuid.UUID, message string) (smsmodule.Message, error)
	MarkSubmitted(ctx context.Context, id uuid.UUID, teamID uuid.UUID, providerID string, providerMessageID string, status string) (smsmodule.Message, error)
}

func (h *Handler) HandleExhausted(ctx context.Context, command DeliverCommand, cause error) error {
	if command.MessageID == uuid.Nil || command.TeamID == uuid.Nil {
		return errors.New("SMS delivery command requires message and team IDs")
	}

	message, err := h.repository.Get(ctx, command.MessageID, command.TeamID)
	if errors.Is(err, smsmodule.ErrMessageNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if message.Status == smsmodule.StatusQueued {
		return fmt.Errorf("SMS delivery retries exhausted before message %s was claimed", message.ID)
	}
	if message.Status != smsmodule.StatusProcessing {
		return nil
	}

	reason := "SMS delivery retries exhausted with an unknown provider outcome"
	if cause != nil {
		reason = fmt.Sprintf("%s: %s", reason, cause)
	}
	_, err = h.repository.MarkDeliveryUnknown(ctx, command.MessageID, command.TeamID, reason)
	if errors.Is(err, smsmodule.ErrMessageNotFound) {
		current, getErr := h.repository.Get(ctx, command.MessageID, command.TeamID)
		if errors.Is(getErr, smsmodule.ErrMessageNotFound) {
			return nil
		}
		if getErr != nil {
			return getErr
		}
		if current.Status == smsmodule.StatusProcessing {
			return errors.New("SMS message remained processing after exhausted delivery finalization")
		}
		return nil
	}
	return err
}

type Handler struct {
	repository           messageRepository
	sender               smsmodule.Sender
	staleProcessingAfter time.Duration
}

func NewHandler(repository *smsmodule.Repository, sender smsmodule.Sender) *Handler {
	return &Handler{repository: repository, sender: sender, staleProcessingAfter: defaultStaleProcessingAfter}
}

func (h *Handler) Handle(ctx context.Context, command DeliverCommand) error {
	if command.MessageID == uuid.Nil || command.TeamID == uuid.Nil {
		return errors.New("SMS delivery command requires message and team IDs")
	}

	message, err := h.repository.MarkProcessing(ctx, command.MessageID, command.TeamID)
	if err != nil {
		if !errors.Is(err, smsmodule.ErrMessageNotFound) {
			return err
		}
		return h.handleAlreadyClaimed(ctx, command)
	}

	response, err := h.sender.Send(ctx, smsapi.SendRequest{
		To:                 message.To,
		From:               message.From,
		Message:            message.Body,
		DestinationCountry: message.DestinationCountry,
	})
	if err != nil {
		if !shouldFinalizeAfterSendError(err) {
			// Ambiguous failures may have happened after the provider accepted the
			// SMS. Keep the message in processing so retries do not re-submit it;
			// stale-processing recovery will eventually close it out operationally.
			return err
		}
		_, updateErr := h.repository.MarkFailed(ctx, command.MessageID, command.TeamID, err.Error())
		return updateErr
	}

	_, err = h.repository.MarkSubmitted(ctx, command.MessageID, command.TeamID, response.ProviderID, response.ProviderMsgID, smsmodule.MapProviderStatus(response.Status))
	return err
}

func (h *Handler) handleAlreadyClaimed(ctx context.Context, command DeliverCommand) error {
	message, err := h.repository.Get(ctx, command.MessageID, command.TeamID)
	if err != nil {
		if errors.Is(err, smsmodule.ErrMessageNotFound) {
			return nil
		}
		return err
	}
	if message.Status != smsmodule.StatusProcessing {
		return nil
	}
	if !h.processingIsStale(message) {
		return fmt.Errorf("sms message %s is already processing", message.ID)
	}
	const reason = "SMS delivery outcome unknown after processing timeout"
	_, updateErr := h.repository.MarkFailed(ctx, command.MessageID, command.TeamID, reason)
	return updateErr
}

func (h *Handler) processingIsStale(message smsmodule.Message) bool {
	threshold := h.staleProcessingAfter
	if threshold <= 0 {
		threshold = defaultStaleProcessingAfter
	}
	return time.Since(message.UpdatedAt) >= threshold
}

type safeFallbackError interface {
	error
	SafeToFallback() bool
}

func shouldFinalizeAfterSendError(err error) bool {
	if err == nil {
		return false
	}
	var validationErr *smsapi.ValidationError
	if errors.As(err, &validationErr) {
		return true
	}
	if errors.Is(err, smsapi.ErrNoProviderAvailable) || errors.Is(err, smsapi.ErrProviderNotFound) {
		return true
	}

	var sendErr *smsapi.SendError
	if errors.As(err, &sendErr) {
		if len(sendErr.Attempts) == 0 {
			return false
		}
		for _, attempt := range sendErr.Attempts {
			if !safeProviderRejection(attempt.Err) {
				return false
			}
		}
		return true
	}

	return safeProviderRejection(err)
}

func safeProviderRejection(err error) bool {
	if err == nil {
		return false
	}
	var fallbackErr safeFallbackError
	return errors.As(err, &fallbackErr) && fallbackErr.SafeToFallback()
}
