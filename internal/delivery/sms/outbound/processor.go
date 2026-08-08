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

func NewProcessor(repository *smsmodule.Repository, sender providerSender) *Processor {
	return &Processor{repository: repository, sender: sender, staleProcessingAfter: defaultStaleProcessingAfter}
}

func NewHandler(repository *smsmodule.Repository, sender providerSender) *Processor {
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

	routes, err := processor.repository.ResolveDeliveryRoutes(ctx, command.MessageID, command.TeamID)
	if err != nil {
		_, updateErr := processor.repository.MarkFailed(
			ctx,
			command.MessageID,
			command.TeamID,
			fmt.Sprintf("resolve canonical SMS route: %v", err),
		)
		return updateErr
	}
	routes = supportedDeliveryRoutes(routes, processor.sender.ProviderIDs())
	if len(routes) == 0 {
		_, updateErr := processor.repository.MarkFailed(
			ctx,
			command.MessageID,
			command.TeamID,
			"no configured provider supports an eligible canonical SMS route",
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
	for index, route := range routes {
		attemptID, attemptErr := processor.repository.CreateDeliveryAttempt(
			ctx,
			command.MessageID,
			command.TeamID,
			route,
		)
		if attemptErr != nil {
			return attemptErr
		}
		if attemptErr := processor.repository.MarkDeliveryAttemptStarted(
			ctx,
			command.MessageID,
			command.TeamID,
			attemptID,
		); attemptErr != nil {
			return attemptErr
		}

		response, sendErr := processor.sender.SendWithProvider(ctx, route.Provider, request)
		if sendErr == nil {
			return processor.repository.MarkDeliveryAttemptSubmitted(
				ctx,
				command.MessageID,
				command.TeamID,
				attemptID,
				response,
			)
		}

		hasFallback := index+1 < len(routes) && processor.sender.ShouldFallback(ctx, route.Provider, sendErr)
		if hasFallback {
			if recordErr := processor.repository.MarkDeliveryAttemptRetryable(
				ctx,
				command.MessageID,
				command.TeamID,
				attemptID,
				sendErr,
			); recordErr != nil {
				return errors.Join(sendErr, recordErr)
			}
			continue
		}
		if shouldFinalizeAfterSendError(sendErr) {
			return processor.repository.MarkDeliveryAttemptFailed(
				ctx,
				command.MessageID,
				command.TeamID,
				attemptID,
				sendErr,
			)
		}
		return processor.repository.MarkDeliveryAttemptUnknown(
			ctx,
			command.MessageID,
			command.TeamID,
			attemptID,
			sendErr,
		)
	}

	_, err = processor.repository.MarkFailed(
		ctx,
		command.MessageID,
		command.TeamID,
		"no eligible canonical SMS route is available",
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
	return processor.repository.FinalizeInFlightDelivery(
		ctx,
		command.MessageID,
		command.TeamID,
		errors.New(reason),
	)
}

func supportedDeliveryRoutes(
	routes []smsmodule.DeliveryRoute,
	providerIDs []string,
) []smsmodule.DeliveryRoute {
	available := make(map[string]struct{}, len(providerIDs))
	for _, providerID := range providerIDs {
		providerID = strings.ToLower(strings.TrimSpace(providerID))
		if providerID != "" {
			available[providerID] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(routes))
	result := make([]smsmodule.DeliveryRoute, 0, len(routes))
	for _, route := range routes {
		providerID := strings.ToLower(strings.TrimSpace(route.Provider))
		if providerID == "" || !strings.EqualFold(strings.TrimSpace(route.ProviderAccount), "default") {
			continue
		}
		if _, ok := available[providerID]; !ok {
			continue
		}
		if _, duplicate := seen[providerID]; duplicate {
			continue
		}
		seen[providerID] = struct{}{}
		result = append(result, route)
	}
	return result
}

func (processor *Processor) processingIsStale(message smsmodule.Message) bool {
	threshold := processor.staleProcessingAfter
	if threshold <= 0 {
		threshold = defaultStaleProcessingAfter
	}
	return time.Since(message.UpdatedAt) >= threshold
}
