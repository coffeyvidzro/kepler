package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	verifychannel "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/channel"
	"github.com/coffeyvidzro/dugble/server/internal/monitoring/verifymetrics"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type codeCipher interface{ Decrypt([]byte) ([]byte, error) }

type channelDispatcher interface {
	DispatchTx(context.Context, pgx.Tx, verifychannel.Input) (platformbilling.CommittedAuthorization, error)
	ObserveCommitted(context.Context, platformbilling.CommittedAuthorization)
}

type Handler struct {
	repository *Repository
	cipher     codeCipher
	email      channelDispatcher
	sms        channelDispatcher
	events     *platformevent.Emitter
	now        func() time.Time
}

func NewHandler(repository *Repository, cipher codeCipher, email, sms channelDispatcher, events *platformevent.Emitter) *Handler {
	return &Handler{repository: repository, cipher: cipher, email: email, sms: sms, events: events, now: time.Now}
}

func (handler *Handler) Handle(ctx context.Context, command Command) (err error) {
	started := time.Now()
	defer func() {
		verifymetrics.Default.Observe("dispatch", verifymetrics.Outcome(err), time.Since(started))
		if err != nil {
			slog.WarnContext(ctx, "verification dispatch failed",
				"verification_id", command.VerificationID,
				"challenge_id", command.ChallengeID,
				"team_id", command.TeamID,
				"error", err,
			)
		}
	}()

	if err := handler.validate(); err != nil {
		return err
	}
	code, err := handler.cipher.Decrypt(command.EncryptedCode)
	if err != nil {
		return fmt.Errorf("decrypt verification code: %w", err)
	}
	defer clear(code)

	tx, err := handler.repository.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin verification dispatch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := handler.repository.Lock(ctx, tx, command)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.ChallengeStatus == "dispatched" || isTerminalChallenge(state.ChallengeStatus) || state.VerificationStatus != "pending" {
		return nil
	}
	if !state.ExpiresAt.After(handler.now().UTC()) {
		if err := handler.repository.MarkExpired(ctx, tx, state); err != nil {
			return err
		}
		snapshot, err := handler.repository.Snapshot(ctx, tx, state.VerificationID, state.TeamID)
		if err != nil {
			return err
		}
		if err := handler.emit(ctx, tx, platformevent.TypeVerificationExpired, snapshot); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if state.ChallengeStatus == "queued" {
		if err := handler.repository.MarkDispatching(ctx, tx, state.ChallengeID, state.TeamID); err != nil {
			return err
		}
	}
	dispatcher, err := handler.dispatcher(state.Channel)
	if err != nil {
		return err
	}
	authorization, err := dispatcher.DispatchTx(ctx, tx, verifychannel.Input{
		TeamID: state.TeamID, VerificationID: state.VerificationID, ChallengeID: state.ChallengeID,
		Recipient: state.Recipient, Code: string(code),
	})
	if err != nil {
		return fmt.Errorf("dispatch verification through %s: %w", state.Channel, err)
	}
	if err := handler.repository.MarkDispatched(ctx, tx, state, authorization.MessageID); err != nil {
		return err
	}
	snapshot, err := handler.repository.Snapshot(ctx, tx, state.VerificationID, state.TeamID)
	if err != nil {
		return err
	}
	if err := handler.emit(ctx, tx, platformevent.TypeVerificationDispatched, snapshot); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit verification dispatch: %w", err)
	}
	dispatcher.ObserveCommitted(ctx, authorization)
	return nil
}

func (handler *Handler) HandleExhausted(ctx context.Context, command Command, cause error) (err error) {
	started := time.Now()
	defer func() {
		verifymetrics.Default.Observe("dispatch_exhausted", verifymetrics.Outcome(err), time.Since(started))
		attributes := []any{
			"verification_id", command.VerificationID,
			"challenge_id", command.ChallengeID,
			"team_id", command.TeamID,
			"cause", cause,
		}
		if err != nil {
			attributes = append(attributes, "error", err)
			slog.ErrorContext(ctx, "verification dispatch exhaustion handling failed", attributes...)
			return
		}
		slog.ErrorContext(ctx, "verification dispatch exhausted", attributes...)
	}()

	if err := handler.validate(); err != nil {
		return err
	}
	tx, err := handler.repository.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin verification failure transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := handler.repository.Lock(ctx, tx, command)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.ChallengeStatus == "dispatched" || isTerminalChallenge(state.ChallengeStatus) || state.VerificationStatus != "pending" {
		return nil
	}
	if err := handler.repository.MarkDeliveryFailed(ctx, tx, state); err != nil {
		return err
	}
	snapshot, err := handler.repository.Snapshot(ctx, tx, state.VerificationID, state.TeamID)
	if err != nil {
		return err
	}
	if err := handler.emit(ctx, tx, platformevent.TypeVerificationDeliveryFailed, snapshot); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (handler *Handler) validate() error {
	if handler == nil || handler.repository == nil || handler.cipher == nil || handler.events == nil {
		return errors.New("verification dispatch handler is not configured")
	}
	if handler.email == nil || handler.sms == nil {
		return errors.New("verification channels are not configured")
	}
	return nil
}

func (handler *Handler) dispatcher(channel string) (channelDispatcher, error) {
	switch channel {
	case "email":
		return handler.email, nil
	case "sms":
		return handler.sms, nil
	default:
		return nil, fmt.Errorf("unsupported verification channel %q", channel)
	}
}

func (handler *Handler) emit(ctx context.Context, tx pgx.Tx, eventType platformevent.Type, snapshot VerificationSnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode verification event: %w", err)
	}
	verificationID, err := uuid.Parse(snapshot.ID)
	if err != nil {
		return err
	}
	teamID, err := uuid.Parse(snapshot.TeamID)
	if err != nil {
		return err
	}
	_, err = handler.events.EmitTx(ctx, tx, platformevent.Envelope{
		Type: eventType, TeamID: teamID, ObjectType: "verification", ObjectID: &verificationID,
		Data: data, OccurredAt: snapshot.UpdatedAt,
	})
	return err
}

func isTerminalChallenge(status string) bool {
	switch status {
	case "delivery_failed", "superseded", "expired":
		return true
	default:
		return false
	}
}
