package smsdelivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const defaultStaleProcessingAfter = 15 * time.Minute

type Processor struct {
	repository           messageRepository
	sender               providerSender
	staleProcessingAfter time.Duration
}

type Handler = Processor

func NewProcessor(repository messageRepository, sender providerSender) *Processor {
	return &Processor{repository: repository, sender: sender, staleProcessingAfter: defaultStaleProcessingAfter}
}

func NewHandler(repository messageRepository, sender providerSender) *Processor {
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
	err = processor.repository.FinalizeInFlightDelivery(
		ctx,
		command.MessageID,
		command.TeamID,
		errors.New(reason),
	)
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

	route, err := processor.repository.ResolveDeliveryRoute(ctx, command.MessageID, command.TeamID)
	if err != nil {
		_, updateErr := processor.repository.MarkFailed(
			ctx,
			command.MessageID,
			command.TeamID,
			fmt.Sprintf("resolve canonical SMS route: %v", err),
		)
		return updateErr
	}
	if !providerAvailable(route.Provider, processor.sender.ProviderIDs()) {
		_, updateErr := processor.repository.MarkFailed(
			ctx,
			command.MessageID,
			command.TeamID,
			"no configured provider supports the canonical SMS route",
		)
		return updateErr
	}

	request := smsapi.SendRequest{
		Reference:          message.ID,
		To:                 message.To,
		From:               message.From,
		Message:            message.Body,
		DestinationCountry: message.DestinationCountry,
	}
	attemptID, err := processor.repository.CreateDeliveryAttempt(
		ctx, command.MessageID, command.TeamID, route,
	)
	if err != nil {
		return err
	}
	if err := processor.repository.MarkDeliveryAttemptStarted(
		ctx, command.MessageID, command.TeamID, attemptID,
	); err != nil {
		return err
	}

	response, sendErr := processor.sender.SendWithProvider(ctx, route.Provider, request)
	if sendErr == nil {
		return processor.repository.MarkDeliveryAttemptSubmitted(
			ctx, command.MessageID, command.TeamID, attemptID, response,
		)
	}
	if shouldFinalizeAfterSendError(sendErr) {
		return processor.repository.MarkDeliveryAttemptFailed(
			ctx, command.MessageID, command.TeamID, attemptID, sendErr,
		)
	}
	return processor.repository.MarkDeliveryAttemptUnknown(
		ctx, command.MessageID, command.TeamID, attemptID, sendErr,
	)
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
	return processor.repository.FinalizeInFlightDelivery(
		ctx,
		command.MessageID,
		command.TeamID,
		errors.New(reason),
	)
}

func providerAvailable(provider string, providerIDs []string) bool {
	for _, providerID := range providerIDs {
		if strings.EqualFold(strings.TrimSpace(providerID), strings.TrimSpace(provider)) {
			return true
		}
	}
	return false
}

func (processor *Processor) processingIsStale(message smsmodule.Message) bool {
	threshold := processor.staleProcessingAfter
	if threshold <= 0 {
		threshold = defaultStaleProcessingAfter
	}
	return time.Since(message.UpdatedAt) >= threshold
}
