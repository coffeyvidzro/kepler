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
	"github.com/jackc/pgx/v5/pgxpool"

	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

type DispatchState struct {
	ChallengeID        uuid.UUID
	VerificationID     uuid.UUID
	TeamID             uuid.UUID
	ChallengeStatus    string
	VerificationStatus string
	Channel            string
	Recipient          string
	ExpiresAt          time.Time
	EmailMessageID     *uuid.UUID
	SMSMessageID       *uuid.UUID
}

type VerificationSnapshot struct {
	ID           string          `json:"id"`
	TeamID       string          `json:"team_id"`
	Channel      string          `json:"channel"`
	Recipient    string          `json:"recipient"`
	CodeLength   int32           `json:"code_length"`
	TTLSeconds   int32           `json:"ttl_seconds"`
	MaxAttempts  int32           `json:"max_attempts"`
	MaxResends   int32           `json:"max_resends"`
	Status       string          `json:"status"`
	Locale       *string         `json:"locale,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	AttemptCount int32           `json:"attempt_count"`
	ResendCount  int32           `json:"resend_count"`
	ExpiresAt    time.Time       `json:"expires_at"`
	ApprovedAt   *time.Time      `json:"approved_at,omitempty"`
	ExpiredAt    *time.Time      `json:"expired_at,omitempty"`
	CanceledAt   *time.Time      `json:"canceled_at,omitempty"`
	FailedAt     *time.Time      `json:"failed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (repository *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	if repository == nil || repository.db == nil {
		return nil, ErrProcessorNotConfigured
	}
	return repository.db.BeginTx(ctx, pgx.TxOptions{})
}

func (repository *Repository) Lock(ctx context.Context, tx pgx.Tx, command Command) (DispatchState, error) {
	var state DispatchState
	state.VerificationID = command.VerificationID
	state.TeamID = command.TeamID
	if err := tx.QueryRow(ctx, `
		SELECT status, recipient
		FROM verifications
		WHERE id = $1 AND team_id = $2
		FOR UPDATE
	`, command.VerificationID, command.TeamID).Scan(
		&state.VerificationStatus,
		&state.Recipient,
	); errors.Is(err, pgx.ErrNoRows) {
		return DispatchState{}, ErrNotFound
	} else if err != nil {
		return DispatchState{}, fmt.Errorf("lock verification dispatch parent: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		SELECT id, verification_id, team_id, status, channel, expires_at,
		       email_message_id, sms_message_id
		FROM verification_challenges
		WHERE id = $1
		  AND verification_id = $2
		  AND team_id = $3
		FOR UPDATE
	`, command.ChallengeID, command.VerificationID, command.TeamID).Scan(
		&state.ChallengeID,
		&state.VerificationID,
		&state.TeamID,
		&state.ChallengeStatus,
		&state.Channel,
		&state.ExpiresAt,
		&state.EmailMessageID,
		&state.SMSMessageID,
	); errors.Is(err, pgx.ErrNoRows) {
		return DispatchState{}, ErrNotFound
	} else if err != nil {
		return DispatchState{}, fmt.Errorf("lock verification dispatch challenge: %w", err)
	}
	return state, nil
}

func (repository *Repository) MarkDispatching(ctx context.Context, tx pgx.Tx, challengeID, teamID uuid.UUID) error {
	result, err := tx.Exec(ctx, `
		UPDATE verification_challenges
		SET status = 'dispatching', updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'queued'
	`, challengeID, teamID)
	if err != nil {
		return fmt.Errorf("mark verification challenge dispatching: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (repository *Repository) MarkDispatched(ctx context.Context, tx pgx.Tx, state DispatchState, messageID uuid.UUID) error {
	column := "email_message_id"
	if state.Channel == "sms" {
		column = "sms_message_id"
	}
	query := `UPDATE verification_challenges SET status = 'dispatched', ` + column + ` = $1, dispatched_at = now(), updated_at = now() WHERE id = $2 AND team_id = $3 AND status IN ('queued','dispatching')`
	result, err := tx.Exec(ctx, query, messageID, state.ChallengeID, state.TeamID)
	if err != nil {
		return fmt.Errorf("mark verification challenge dispatched: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (repository *Repository) MarkExpired(ctx context.Context, tx pgx.Tx, state DispatchState) error {
	if _, err := tx.Exec(ctx, `
		UPDATE verification_challenges
		SET status = 'expired', updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status IN ('queued','dispatching')
	`, state.ChallengeID, state.TeamID); err != nil {
		return fmt.Errorf("expire verification challenge: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE verifications
		SET status = 'expired', expired_at = now(), updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'pending'
	`, state.VerificationID, state.TeamID); err != nil {
		return fmt.Errorf("expire verification: %w", err)
	}
	return nil
}

func (repository *Repository) MarkDeliveryFailed(ctx context.Context, tx pgx.Tx, state DispatchState) error {
	if _, err := tx.Exec(ctx, `
		UPDATE verification_challenges
		SET status = 'delivery_failed', delivery_failed_at = now(), updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status IN ('queued','dispatching')
	`, state.ChallengeID, state.TeamID); err != nil {
		return fmt.Errorf("mark verification challenge delivery failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE verifications
		SET status = 'delivery_failed', failed_at = now(), updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'pending'
	`, state.VerificationID, state.TeamID); err != nil {
		return fmt.Errorf("mark verification delivery failed: %w", err)
	}
	return nil
}

func (repository *Repository) Snapshot(ctx context.Context, tx pgx.Tx, verificationID, teamID uuid.UUID) (VerificationSnapshot, error) {
	var snapshot VerificationSnapshot
	var id uuid.UUID
	var metadata []byte
	err := tx.QueryRow(ctx, `
		SELECT id, channel, recipient, code_length, ttl_seconds, max_attempts,
		       max_resends, status, locale, metadata, attempt_count, resend_count,
		       expires_at, approved_at, expired_at, canceled_at, failed_at,
		       created_at, updated_at
		FROM verifications
		WHERE id = $1 AND team_id = $2
	`, verificationID, teamID).Scan(
		&id, &snapshot.Channel, &snapshot.Recipient, &snapshot.CodeLength,
		&snapshot.TTLSeconds, &snapshot.MaxAttempts, &snapshot.MaxResends,
		&snapshot.Status, &snapshot.Locale, &metadata, &snapshot.AttemptCount,
		&snapshot.ResendCount, &snapshot.ExpiresAt, &snapshot.ApprovedAt,
		&snapshot.ExpiredAt, &snapshot.CanceledAt, &snapshot.FailedAt,
		&snapshot.CreatedAt, &snapshot.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationSnapshot{}, ErrNotFound
	}
	if err != nil {
		return VerificationSnapshot{}, fmt.Errorf("read verification snapshot: %w", err)
	}
	snapshot.ID = id.String()
	snapshot.TeamID = teamID.String()
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	snapshot.Metadata = metadata
	return snapshot, nil
}

type ChannelInput struct {
	TeamID         uuid.UUID
	VerificationID uuid.UUID
	ChallengeID    uuid.UUID
	Recipient      string
	Code           string
}

type channelDispatcher interface {
	DispatchTx(context.Context, pgx.Tx, ChannelInput) (platformbilling.CommittedAuthorization, error)
	ObserveCommitted(context.Context, platformbilling.CommittedAuthorization)
}

type EmailChannel struct {
	service *emailmodule.Service
}

func NewEmailChannel(service *emailmodule.Service) *EmailChannel {
	return &EmailChannel{service: service}
}

func (channel *EmailChannel) DispatchTx(ctx context.Context, tx pgx.Tx, input ChannelInput) (platformbilling.CommittedAuthorization, error) {
	return channel.service.EnqueueVerificationTx(ctx, tx, emailmodule.VerificationEmailInput{
		TeamID: input.TeamID, VerificationID: input.VerificationID, ChallengeID: input.ChallengeID,
		Recipient: input.Recipient, Code: input.Code,
	})
}

func (channel *EmailChannel) ObserveCommitted(ctx context.Context, authorization platformbilling.CommittedAuthorization) {
	channel.service.ObserveVerificationCommitted(ctx, authorization)
}

type SMSChannel struct {
	service *smsmodule.Service
	sender  string
}

func NewSMSChannel(service *smsmodule.Service, sender string) *SMSChannel {
	return &SMSChannel{service: service, sender: sender}
}

func (channel *SMSChannel) DispatchTx(ctx context.Context, tx pgx.Tx, input ChannelInput) (platformbilling.CommittedAuthorization, error) {
	return channel.service.EnqueueVerificationTx(ctx, tx, smsmodule.VerificationSMSInput{
		TeamID: input.TeamID, VerificationID: input.VerificationID, ChallengeID: input.ChallengeID,
		Recipient: input.Recipient, Sender: channel.sender, Code: input.Code,
	})
}

func (channel *SMSChannel) ObserveCommitted(ctx context.Context, authorization platformbilling.CommittedAuthorization) {
	channel.service.ObserveVerificationCommitted(ctx, authorization)
}

type codeCipher interface {
	Decrypt([]byte) ([]byte, error)
}

type Processor struct {
	repository *Repository
	cipher     codeCipher
	email      channelDispatcher
	sms        channelDispatcher
	events     *platformevent.Emitter
	now        func() time.Time
}

type Handler = Processor

func NewProcessor(repository *Repository, cipher codeCipher, email, sms channelDispatcher, events *platformevent.Emitter) *Processor {
	return &Processor{repository: repository, cipher: cipher, email: email, sms: sms, events: events, now: time.Now}
}

func NewHandler(repository *Repository, cipher codeCipher, email, sms channelDispatcher, events *platformevent.Emitter) *Processor {
	return NewProcessor(repository, cipher, email, sms, events)
}

func (processor *Processor) Handle(ctx context.Context, command Command) (err error) {
	defer func() {
		if err != nil {
			slog.WarnContext(ctx, "verification dispatch failed",
				"verification_id", command.VerificationID,
				"challenge_id", command.ChallengeID,
				"team_id", command.TeamID,
				"error", err,
			)
		}
	}()
	if err := processor.validate(); err != nil {
		return err
	}
	if err := ValidateCommand(command); err != nil {
		return err
	}
	code, err := processor.cipher.Decrypt(command.EncryptedCode)
	if err != nil {
		return fmt.Errorf("decrypt verification code: %w", err)
	}
	defer clear(code)

	tx, err := processor.repository.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin verification dispatch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := processor.repository.Lock(ctx, tx, command)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.ChallengeStatus == "dispatched" || isTerminalChallenge(state.ChallengeStatus) || state.VerificationStatus != "pending" {
		return nil
	}
	if !state.ExpiresAt.After(processor.now().UTC()) {
		if err := processor.repository.MarkExpired(ctx, tx, state); err != nil {
			return err
		}
		snapshot, err := processor.repository.Snapshot(ctx, tx, state.VerificationID, state.TeamID)
		if err != nil {
			return err
		}
		if err := processor.emit(ctx, tx, platformevent.TypeVerificationExpired, snapshot); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if state.ChallengeStatus == "queued" {
		if err := processor.repository.MarkDispatching(ctx, tx, state.ChallengeID, state.TeamID); err != nil {
			return err
		}
	}
	dispatcher, err := processor.dispatcher(state.Channel)
	if err != nil {
		return err
	}
	authorization, err := dispatcher.DispatchTx(ctx, tx, ChannelInput{
		TeamID: state.TeamID, VerificationID: state.VerificationID, ChallengeID: state.ChallengeID,
		Recipient: state.Recipient, Code: string(code),
	})
	if err != nil {
		return fmt.Errorf("dispatch verification through %s: %w", state.Channel, err)
	}
	if err := processor.repository.MarkDispatched(ctx, tx, state, authorization.MessageID); err != nil {
		return err
	}
	snapshot, err := processor.repository.Snapshot(ctx, tx, state.VerificationID, state.TeamID)
	if err != nil {
		return err
	}
	if err := processor.emit(ctx, tx, platformevent.TypeVerificationDispatched, snapshot); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit verification dispatch: %w", err)
	}
	dispatcher.ObserveCommitted(ctx, authorization)
	return nil
}

func (processor *Processor) HandleExhausted(ctx context.Context, command Command, cause error) (err error) {
	if err := processor.validate(); err != nil {
		return err
	}
	tx, err := processor.repository.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin verification failure transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	state, err := processor.repository.Lock(ctx, tx, command)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if state.ChallengeStatus == "dispatched" || isTerminalChallenge(state.ChallengeStatus) || state.VerificationStatus != "pending" {
		return nil
	}
	if err := processor.repository.MarkDeliveryFailed(ctx, tx, state); err != nil {
		return err
	}
	snapshot, err := processor.repository.Snapshot(ctx, tx, state.VerificationID, state.TeamID)
	if err != nil {
		return err
	}
	if err := processor.emit(ctx, tx, platformevent.TypeVerificationDeliveryFailed, snapshot); err != nil {
		return err
	}
	if cause != nil {
		slog.ErrorContext(ctx, "verification dispatch exhausted", "challenge_id", command.ChallengeID, "cause", cause)
	}
	return tx.Commit(ctx)
}

func (processor *Processor) validate() error {
	if processor == nil || processor.repository == nil || processor.cipher == nil || processor.events == nil {
		return ErrProcessorNotConfigured
	}
	if processor.email == nil || processor.sms == nil {
		return errors.New("verification channels are not configured")
	}
	return nil
}

func (processor *Processor) dispatcher(channel string) (channelDispatcher, error) {
	switch channel {
	case "email":
		return processor.email, nil
	case "sms":
		return processor.sms, nil
	default:
		return nil, fmt.Errorf("unsupported verification channel %q", channel)
	}
}

func (processor *Processor) emit(ctx context.Context, tx pgx.Tx, eventType platformevent.Type, snapshot VerificationSnapshot) error {
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
	_, err = processor.events.EmitTx(ctx, tx, platformevent.Envelope{
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
