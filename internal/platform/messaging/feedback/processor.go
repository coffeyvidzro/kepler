package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/delivery"
)

var ErrInvalidTransition = errors.New("provider feedback would move the delivery attempt to an invalid status")

// Result summarizes one normalized provider-event application.
type Result struct {
	AttemptID      uuid.UUID
	PreviousStatus delivery.AttemptStatus
	Status         delivery.AttemptStatus
	Applied        bool
	Duplicate      bool
}

// Processor validates provider feedback and applies monotonic attempt updates.
type Processor struct {
	repository Repository
}

func NewProcessor(repository Repository) (*Processor, error) {
	if repository == nil {
		return nil, errors.New("feedback repository is required")
	}
	return &Processor{repository: repository}, nil
}

func (processor *Processor) Process(ctx context.Context, event Event) (Result, error) {
	if processor == nil || processor.repository == nil {
		return Result{}, errors.New("feedback processor is not configured")
	}
	if err := event.Validate(); err != nil {
		return Result{}, err
	}

	attempt, err := processor.repository.FindAttempt(ctx, Lookup{
		Provider:          event.Provider,
		ProviderMessageID: event.ProviderMessageID,
		Channel:           event.Channel,
	})
	if err != nil {
		return Result{}, fmt.Errorf("find delivery attempt for feedback: %w", err)
	}
	if attempt.ID == uuid.Nil {
		return Result{}, ErrAttemptNotFound
	}
	if attempt.Channel != event.Channel {
		return Result{}, errors.New("feedback channel does not match delivery attempt")
	}
	if attempt.Provider != "" && !strings.EqualFold(attempt.Provider, event.Provider) {
		return Result{}, errors.New("feedback provider does not match delivery attempt")
	}
	if !attempt.Status.CanTransitionTo(event.Status) {
		return Result{}, fmt.Errorf("%w: %s to %s", ErrInvalidTransition, attempt.Status, event.Status)
	}

	var terminalAt *time.Time
	if event.Status.Terminal() {
		occurredAt := event.OccurredAt
		terminalAt = &occurredAt
	}
	applyResult, err := processor.repository.ApplyEvent(ctx, event, AttemptUpdate{
		AttemptID:      attempt.ID,
		ExpectedStatus: attempt.Status,
		Status:         event.Status,
		ProviderStatus: event.ProviderStatus,
		ErrorCode:      event.ErrorCode,
		ErrorMessage:   event.ErrorMessage,
		OccurredAt:     event.OccurredAt,
		TerminalAt:     terminalAt,
		ReconciledAt:   event.ReceivedAt,
	})
	if err != nil {
		return Result{}, fmt.Errorf("apply provider feedback: %w", err)
	}

	return Result{
		AttemptID:      attempt.ID,
		PreviousStatus: attempt.Status,
		Status:         event.Status,
		Applied:        applyResult.Applied,
		Duplicate:      applyResult.Duplicate,
	}, nil
}
