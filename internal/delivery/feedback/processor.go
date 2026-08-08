package feedback

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/delivery/attempt"
)

// Result summarizes one normalized provider-event application.
type Result struct {
	AttemptID      uuid.UUID
	PreviousStatus attempt.AttemptStatus
	Status         attempt.AttemptStatus
	Applied        bool
	Duplicate      bool
	Transitioned   bool
	Ignored        bool
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
		AttemptID:         event.AttemptID,
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

	update := AttemptUpdate{
		AttemptID:      attempt.ID,
		ExpectedStatus: attempt.Status,
		ProviderStatus: event.ProviderStatus,
		ErrorCode:      event.ErrorCode,
		ErrorMessage:   event.ErrorMessage,
		OccurredAt:     event.OccurredAt,
		ReconciledAt:   event.ReceivedAt,
	}
	status := attempt.Status
	ignored := false
	if attempt.Status.CanTransitionTo(event.Status) {
		nextStatus := event.Status
		update.Status = &nextStatus
		status = nextStatus
		if event.Status.Terminal() {
			occurredAt := event.OccurredAt
			update.TerminalAt = &occurredAt
		}
	} else {
		// Provider callbacks can arrive late or out of order. Record the
		// observation for idempotency and diagnostics without moving the
		// canonical attempt backward or reopening a terminal attempt.
		ignored = true
	}

	applyResult, err := processor.repository.ApplyEvent(ctx, event, update)
	if err != nil {
		return Result{}, fmt.Errorf("apply provider feedback: %w", err)
	}

	return Result{
		AttemptID:      attempt.ID,
		PreviousStatus: attempt.Status,
		Status:         status,
		Applied:        applyResult.Applied,
		Duplicate:      applyResult.Duplicate,
		Transitioned:   applyResult.Transitioned,
		Ignored:        ignored,
	}, nil
}
