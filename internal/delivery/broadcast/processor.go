package broadcastexecution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	broadcastmodule "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	messagetemplate "github.com/coffeyvidzro/dugble/server/internal/modules/messagetemplate"
)

type repository interface {
	QueueNextDueScheduled(context.Context) (broadcastmodule.Broadcast, bool, error)
	MaterializeNextQueuedRecipients(context.Context) (broadcastmodule.MaterializationResult, bool, error)
}

type fanoutRepository interface {
	BeginFanoutTx(context.Context) (pgx.Tx, error)
	ClaimNextRecipientForFanoutTx(context.Context, pgx.Tx) (broadcastmodule.FanoutRecipient, bool, error)
	SetRecipientQueuedTx(context.Context, pgx.Tx, broadcastmodule.FanoutRecipient, uuid.UUID) error
	RetryRecipientFanoutTx(context.Context, pgx.Tx, broadcastmodule.FanoutRecipient, time.Time, string, string) error
	FailRecipientFanoutTx(context.Context, pgx.Tx, broadcastmodule.FanoutRecipient, string, string) error
	FinalizeBroadcastFanoutTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID) (broadcastmodule.Broadcast, error)
}

type templateRenderer interface {
	RenderVersionTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, uuid.UUID, map[string]any) (messagetemplate.PreviewResponse, error)
}

type emailEnqueuer interface {
	EnqueueTx(context.Context, pgx.Tx, uuid.UUID, emailmodule.SendRequest) (emailmodule.QueuedMessage, error)
	ObserveCommitted(context.Context, emailmodule.QueuedMessage)
}

type clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type Processor struct {
	repository repository
	fanout     fanoutRepository
	templates  templateRenderer
	emails     emailEnqueuer
	clock      clock
	retry      RetryPolicy
}

func NewProcessor(repository repository, dependencies ...any) *Processor {
	processor := &Processor{
		repository: repository,
		clock:      systemClock{},
		retry:      DefaultRetryPolicy(),
	}
	if fanout, ok := repository.(fanoutRepository); ok {
		processor.fanout = fanout
	}
	for _, dependency := range dependencies {
		if renderer, ok := dependency.(templateRenderer); ok {
			processor.templates = renderer
		}
		if enqueuer, ok := dependency.(emailEnqueuer); ok {
			processor.emails = enqueuer
		}
		if value, ok := dependency.(clock); ok {
			processor.clock = value
		}
		if policy, ok := dependency.(RetryPolicy); ok {
			processor.retry = policy.normalized()
		}
	}
	return processor
}

func (p *Processor) ProcessBatch(ctx context.Context, batchSize int) error {
	if p == nil || p.repository == nil {
		return ErrProcessorNotConfigured
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if err := p.queueDueBroadcasts(ctx, batchSize); err != nil {
		return err
	}
	if err := p.materializeQueuedBroadcasts(ctx, batchSize); err != nil {
		return err
	}
	if !p.fanoutConfigured() {
		return nil
	}
	return p.fanoutRecipients(ctx, batchSize)
}

func (p *Processor) fanoutConfigured() bool {
	return p != nil && p.fanout != nil && p.templates != nil && p.emails != nil && p.clock != nil
}

func (p *Processor) queueDueBroadcasts(ctx context.Context, batchSize int) error {
	for processed := 0; processed < batchSize; processed++ {
		broadcast, claimed, err := p.repository.QueueNextDueScheduled(ctx)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		slog.Info(
			"scheduled broadcast queued for execution",
			"broadcast_id", broadcast.ID,
			"team_id", broadcast.TeamID,
			"scheduled_at", broadcast.ScheduledAt,
		)
	}
	return nil
}

func (p *Processor) materializeQueuedBroadcasts(ctx context.Context, batchSize int) error {
	for processed := 0; processed < batchSize; processed++ {
		result, claimed, err := p.repository.MaterializeNextQueuedRecipients(ctx)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		slog.Info(
			"broadcast recipients materialized",
			"broadcast_id", result.BroadcastID,
			"team_id", result.TeamID,
			"audience_count", result.AudienceCount,
			"eligible_count", result.EligibleCount,
			"excluded_count", result.ExcludedCount,
		)
	}
	return nil
}

func (p *Processor) fanoutRecipients(ctx context.Context, batchSize int) error {
	for processed := 0; processed < batchSize; processed++ {
		claimed, err := p.processNextRecipient(ctx)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
	}
	return nil
}

func (p *Processor) processNextRecipient(ctx context.Context) (bool, error) {
	tx, err := p.fanout.BeginFanoutTx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	recipient, claimed, err := p.fanout.ClaimNextRecipientForFanoutTx(ctx, tx)
	if err != nil || !claimed {
		return claimed, err
	}

	variables := resolveRecipientVariables(recipient)
	rendered, err := p.templates.RenderVersionTx(
		ctx,
		tx,
		recipient.TeamID,
		recipient.TemplateID,
		recipient.TemplateVersionID,
		variables,
	)
	if err != nil {
		return p.recordRecipientFailure(ctx, tx, recipient, classifyRenderFailure(err))
	}

	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin email fanout savepoint: %w", err)
	}
	queued, err := p.emails.EnqueueTx(ctx, savepoint, recipient.TeamID, buildEmailRequest(recipient, rendered))
	if err != nil {
		if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return false, fmt.Errorf("rollback email fanout savepoint: %w", rollbackErr)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return p.recordRecipientFailure(ctx, tx, recipient, classifyFanoutFailure(err))
	}
	if err := savepoint.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit email fanout savepoint: %w", err)
	}

	messageID, err := uuid.Parse(queued.Message.ID)
	if err != nil {
		return false, fmt.Errorf("parse queued email message id: %w", err)
	}
	if err := p.fanout.SetRecipientQueuedTx(ctx, tx, recipient, messageID); err != nil {
		return false, err
	}
	broadcast, err := p.fanout.FinalizeBroadcastFanoutTx(ctx, tx, recipient.TeamID, recipient.BroadcastID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit broadcast recipient fanout: %w", err)
	}
	p.emails.ObserveCommitted(ctx, queued)
	slog.Info(
		"broadcast recipient queued for email delivery",
		"broadcast_id", recipient.BroadcastID,
		"recipient_id", recipient.ID,
		"email_message_id", messageID,
		"broadcast_status", broadcast.Status,
	)
	return true, nil
}

func (p *Processor) recordRecipientFailure(ctx context.Context, tx pgx.Tx, recipient broadcastmodule.FanoutRecipient, failure fanoutFailure) (bool, error) {
	if errors.Is(failure.cause, context.Canceled) || errors.Is(failure.cause, context.DeadlineExceeded) {
		return false, failure.cause
	}

	attempt := recipient.AttemptCount + 1
	retryable := failure.retryable && attempt < p.retry.MaxAttempts
	if retryable {
		nextAttemptAt := p.clock.Now().Add(p.retry.delay(recipient.AttemptCount))
		if err := p.fanout.RetryRecipientFanoutTx(ctx, tx, recipient, nextAttemptAt, failure.code, failure.message); err != nil {
			return false, err
		}
	} else {
		if err := p.fanout.FailRecipientFanoutTx(ctx, tx, recipient, failure.code, failure.message); err != nil {
			return false, err
		}
	}

	broadcast, err := p.fanout.FinalizeBroadcastFanoutTx(ctx, tx, recipient.TeamID, recipient.BroadcastID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit broadcast recipient failure: %w", err)
	}
	slog.Warn(
		"broadcast recipient fanout failed",
		"broadcast_id", recipient.BroadcastID,
		"recipient_id", recipient.ID,
		"attempt", attempt,
		"retryable", retryable,
		"error_code", failure.code,
		"broadcast_status", broadcast.Status,
	)
	return true, nil
}

func resolveRecipientVariables(recipient broadcastmodule.FanoutRecipient) map[string]any {
	properties, _ := recipient.ContactSnapshot["properties"].(map[string]any)
	variables := make(map[string]any, len(properties)+len(recipient.VariableBindings)+3)
	for key, value := range properties {
		variables[key] = value
	}
	variables["EMAIL"] = recipient.Email
	variables["FIRST_NAME"] = pointerString(recipient.FirstName)
	variables["LAST_NAME"] = pointerString(recipient.LastName)
	for key, value := range recipient.VariableBindings {
		variables[key] = value
	}
	return variables
}

func buildEmailRequest(recipient broadcastmodule.FanoutRecipient, rendered messagetemplate.PreviewResponse) emailmodule.SendRequest {
	request := emailmodule.SendRequest{
		Stream:  emailmodule.MessageTypeMarketing,
		To:      emailmodule.EmailAddressList{{Email: recipient.Email, Name: recipientName(recipient)}},
		Subject: rendered.Subject,
		HTML:    rendered.HTML,
	}
	if rendered.Text != nil {
		request.Text = *rendered.Text
	}
	if rendered.FromEmail != nil {
		request.From = &emailmodule.EmailAddress{Email: *rendered.FromEmail}
		if rendered.FromName != nil {
			request.From.Name = *rendered.FromName
		}
	}
	if rendered.ReplyTo != nil {
		request.ReplyTo = emailmodule.EmailAddressList{{Email: *rendered.ReplyTo}}
	}
	return request
}

func recipientName(recipient broadcastmodule.FanoutRecipient) string {
	return strings.TrimSpace(strings.TrimSpace(pointerString(recipient.FirstName)) + " " + strings.TrimSpace(pointerString(recipient.LastName)))
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
