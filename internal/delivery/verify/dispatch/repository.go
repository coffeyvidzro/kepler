package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("verification dispatch resource not found")

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
	ServiceID    string          `json:"service_id"`
	Channel      string          `json:"channel"`
	Recipient    string          `json:"recipient"`
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

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (repository *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
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
	result, err := tx.Exec(ctx, `UPDATE verification_challenges SET status = 'dispatching', updated_at = now() WHERE id = $1 AND team_id = $2 AND status = 'queued'`, challengeID, teamID)
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
	if _, err := tx.Exec(ctx, `UPDATE verification_challenges SET status = 'expired', updated_at = now() WHERE id = $1 AND team_id = $2 AND status IN ('queued','dispatching')`, state.ChallengeID, state.TeamID); err != nil {
		return fmt.Errorf("expire verification challenge: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE verifications SET status = 'expired', expired_at = now(), updated_at = now() WHERE id = $1 AND team_id = $2 AND status = 'pending'`, state.VerificationID, state.TeamID); err != nil {
		return fmt.Errorf("expire verification: %w", err)
	}
	return nil
}

func (repository *Repository) MarkDeliveryFailed(ctx context.Context, tx pgx.Tx, state DispatchState) error {
	if _, err := tx.Exec(ctx, `UPDATE verification_challenges SET status = 'delivery_failed', delivery_failed_at = now(), updated_at = now() WHERE id = $1 AND team_id = $2 AND status IN ('queued','dispatching')`, state.ChallengeID, state.TeamID); err != nil {
		return fmt.Errorf("mark verification challenge delivery failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE verifications SET status = 'delivery_failed', failed_at = now(), updated_at = now() WHERE id = $1 AND team_id = $2 AND status = 'pending'`, state.VerificationID, state.TeamID); err != nil {
		return fmt.Errorf("mark verification delivery failed: %w", err)
	}
	return nil
}

func (repository *Repository) Snapshot(ctx context.Context, tx pgx.Tx, verificationID, teamID uuid.UUID) (VerificationSnapshot, error) {
	var snapshot VerificationSnapshot
	var id, serviceID uuid.UUID
	var metadata []byte
	err := tx.QueryRow(ctx, `
		SELECT id, service_id, channel, recipient, status, locale, metadata,
		       attempt_count, resend_count, expires_at, approved_at, expired_at,
		       canceled_at, failed_at, created_at, updated_at
		FROM verifications WHERE id = $1 AND team_id = $2
	`, verificationID, teamID).Scan(
		&id, &serviceID, &snapshot.Channel, &snapshot.Recipient, &snapshot.Status,
		&snapshot.Locale, &metadata, &snapshot.AttemptCount, &snapshot.ResendCount,
		&snapshot.ExpiresAt, &snapshot.ApprovedAt, &snapshot.ExpiredAt,
		&snapshot.CanceledAt, &snapshot.FailedAt, &snapshot.CreatedAt, &snapshot.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationSnapshot{}, ErrNotFound
	}
	if err != nil {
		return VerificationSnapshot{}, fmt.Errorf("read verification snapshot: %w", err)
	}
	snapshot.ID, snapshot.TeamID, snapshot.ServiceID = id.String(), teamID.String(), serviceID.String()
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	snapshot.Metadata = metadata
	return snapshot, nil
}
