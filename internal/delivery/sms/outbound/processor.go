package smsdelivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const defaultStaleProcessingAfter = 15 * time.Minute

type Processor struct {
	repository           messageRepository
	sender               smsmodule.Sender
	staleProcessingAfter time.Duration
}

type Handler = Processor

func NewProcessor(repository *smsmodule.Repository, sender smsmodule.Sender) *Processor {
	return &Processor{repository: repository, sender: sender, staleProcessingAfter: defaultStaleProcessingAfter}
}

func NewHandler(repository *smsmodule.Repository, sender smsmodule.Sender) *Processor {
	return NewProcessor(repository, sender)
}

func (processor *Processor) HandleExhausted(ctx context.Context, command DeliverCommand, cause error) error {
	if processor == nil || processor.repository == nil {
		return ErrProcessorNotConfigured
	}
	if command.MessageID == uuid.Nil || command.TeamID == uuid.Nil {
		return errors.New("SMS delivery command requires message and team IDs")
	}

	message, err := processor.repository.Get(ctx, command.MessageID, command.TeamID)
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
	_, err = processor.repository.MarkDeliveryUnknown(ctx, command.MessageID, command.TeamID, reason)
	if errors.Is(err, smsmodule.ErrMessageNotFound) {
		current, getErr := processor.repository.Get(ctx, command.MessageID, command.TeamID)
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

func (processor *Processor) Handle(ctx context.Context, command DeliverCommand) error {
	if processor == nil || processor.repository == nil || processor.sender == nil {
		return ErrProcessorNotConfigured
	}
	if command.MessageID == uuid.Nil || command.TeamID == uuid.Nil {
		return errors.New("SMS delivery command requires message and team IDs")
	}

	message, err := processor.repository.MarkProcessing(ctx, command.MessageID, command.TeamID)
	if err != nil {
		if !errors.Is(err, smsmodule.ErrMessageNotFound) {
			return err
		}
		return processor.handleAlreadyClaimed(ctx, command)
	}

	response, err := processor.sender.Send(ctx, smsapi.SendRequest{
		To:                 message.To,
		From:               message.From,
		Message:            message.Body,
		DestinationCountry: message.DestinationCountry,
	})
	if err != nil {
		if !shouldFinalizeAfterSendError(err) {
			return err
		}
		_, updateErr := processor.repository.MarkFailed(ctx, command.MessageID, command.TeamID, err.Error())
		return updateErr
	}

	_, err = processor.repository.MarkSubmitted(
		ctx,
		command.MessageID,
		command.TeamID,
		response.ProviderID,
		response.ProviderMsgID,
		smsmodule.MapProviderStatus(response.Status),
	)
	return err
}

func (processor *Processor) handleAlreadyClaimed(ctx context.Context, command DeliverCommand) error {
	message, err := processor.repository.Get(ctx, command.MessageID, command.TeamID)
	if err != nil {
		if errors.Is(err, smsmodule.ErrMessageNotFound) {
			return nil
		}
		return err
	}
	if message.Status != smsmodule.StatusProcessing {
		return nil
	}
	if !processor.processingIsStale(message) {
		return fmt.Errorf("sms message %s is already processing", message.ID)
	}
	const reason = "SMS delivery outcome unknown after processing timeout"
	_, updateErr := processor.repository.MarkFailed(ctx, command.MessageID, command.TeamID, reason)
	return updateErr
}

func (processor *Processor) processingIsStale(message smsmodule.Message) bool {
	threshold := processor.staleProcessingAfter
	if threshold <= 0 {
		threshold = defaultStaleProcessingAfter
	}
	return time.Since(message.UpdatedAt) >= threshold
}
