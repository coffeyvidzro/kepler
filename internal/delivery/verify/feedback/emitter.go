package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

type webhookEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformwebhook.Event) (uuid.UUID, int64, error)
}

type Emitter struct {
	next   webhookEmitter
	events *platformevent.Emitter
}

type verificationSnapshot struct {
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

func NewEmitter(next webhookEmitter, events *platformevent.Emitter) *Emitter {
	return &Emitter{next: next, events: events}
}

func (emitter *Emitter) EmitTx(ctx context.Context, tx pgx.Tx, event platformwebhook.Event) (uuid.UUID, int64, error) {
	if emitter == nil || emitter.next == nil || emitter.events == nil {
		return uuid.Nil, 0, errors.New("verification delivery feedback emitter is not configured")
	}

	eventID, deliveryCount, err := emitter.next.EmitTx(ctx, tx, event)
	if err != nil {
		return uuid.Nil, 0, err
	}

	channel, terminal := terminalFailureChannel(event.Type)
	if !terminal || event.ObjectID == nil || *event.ObjectID == uuid.Nil {
		return eventID, deliveryCount, nil
	}
	if err := emitter.reconcile(ctx, tx, channel, event.TeamID, *event.ObjectID); err != nil {
		return eventID, deliveryCount, fmt.Errorf("reconcile verification %s delivery failure: %w", channel, err)
	}
	return eventID, deliveryCount, nil
}

func (emitter *Emitter) reconcile(ctx context.Context, tx pgx.Tx, channel string, teamID, messageID uuid.UUID) error {
	if tx == nil {
		return errors.New("verification delivery feedback transaction is required")
	}
	if teamID == uuid.Nil || messageID == uuid.Nil {
		return errors.New("verification delivery feedback requires team and message IDs")
	}

	messageColumn, err := channelMessageColumn(channel)
	if err != nil {
		return err
	}

	var verificationID uuid.UUID
	findQuery := `
		SELECT verification_id
		FROM verification_challenges
		WHERE team_id = $1 AND ` + messageColumn + ` = $2
		ORDER BY sequence DESC
		LIMIT 1
	`
	if err := tx.QueryRow(ctx, findQuery, teamID, messageID).Scan(&verificationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("find verification challenge by %s message: %w", channel, err)
	}

	snapshot, err := lockVerification(ctx, tx, verificationID, teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if snapshot.Status != "pending" {
		return nil
	}

	var challengeID uuid.UUID
	var challengeStatus string
	lockChallengeQuery := `
		SELECT id, status
		FROM verification_challenges
		WHERE verification_id = $1
		  AND team_id = $2
		  AND channel = $3
		  AND ` + messageColumn + ` = $4
		FOR UPDATE
	`
	if err := tx.QueryRow(ctx, lockChallengeQuery, verificationID, teamID, channel, messageID).Scan(
		&challengeID,
		&challengeStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("lock verification challenge by %s message: %w", channel, err)
	}
	if challengeStatus != "dispatched" {
		return nil
	}

	challengeResult, err := tx.Exec(ctx, `
		UPDATE verification_challenges
		SET status = 'delivery_failed',
		    delivery_failed_at = COALESCE(delivery_failed_at, now()),
		    updated_at = now()
		WHERE id = $1
		  AND team_id = $2
		  AND status = 'dispatched'
	`, challengeID, teamID)
	if err != nil {
		return fmt.Errorf("mark verification challenge delivery failed: %w", err)
	}
	if challengeResult.RowsAffected() != 1 {
		return nil
	}

	var failedAt, updatedAt time.Time
	if err := tx.QueryRow(ctx, `
		UPDATE verifications
		SET status = 'delivery_failed',
		    failed_at = COALESCE(failed_at, now()),
		    updated_at = now()
		WHERE id = $1
		  AND team_id = $2
		  AND status = 'pending'
		RETURNING failed_at, updated_at
	`, verificationID, teamID).Scan(&failedAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("mark verification delivery failed: %w", err)
	}

	snapshot.Status = "delivery_failed"
	snapshot.FailedAt = &failedAt
	snapshot.UpdatedAt = updatedAt
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode verification delivery failure event: %w", err)
	}
	if _, err := emitter.events.EmitTx(ctx, tx, platformevent.Envelope{
		Type:       platformevent.TypeVerificationDeliveryFailed,
		TeamID:     teamID,
		ObjectType: "verification",
		ObjectID:   &verificationID,
		Data:       data,
		OccurredAt: updatedAt,
	}); err != nil {
		return fmt.Errorf("emit verification delivery failure event: %w", err)
	}
	return nil
}

func lockVerification(ctx context.Context, tx pgx.Tx, verificationID, teamID uuid.UUID) (verificationSnapshot, error) {
	var snapshot verificationSnapshot
	var id, snapshotTeamID, serviceID uuid.UUID
	var metadata []byte
	if err := tx.QueryRow(ctx, `
		SELECT id, team_id, service_id, channel, recipient, status, locale, metadata,
		       attempt_count, resend_count, expires_at, approved_at, expired_at,
		       canceled_at, failed_at, created_at, updated_at
		FROM verifications
		WHERE id = $1 AND team_id = $2
		FOR UPDATE
	`, verificationID, teamID).Scan(
		&id,
		&snapshotTeamID,
		&serviceID,
		&snapshot.Channel,
		&snapshot.Recipient,
		&snapshot.Status,
		&snapshot.Locale,
		&metadata,
		&snapshot.AttemptCount,
		&snapshot.ResendCount,
		&snapshot.ExpiresAt,
		&snapshot.ApprovedAt,
		&snapshot.ExpiredAt,
		&snapshot.CanceledAt,
		&snapshot.FailedAt,
		&snapshot.CreatedAt,
		&snapshot.UpdatedAt,
	); err != nil {
		return verificationSnapshot{}, fmt.Errorf("lock verification for delivery feedback: %w", err)
	}
	snapshot.ID = id.String()
	snapshot.TeamID = snapshotTeamID.String()
	snapshot.ServiceID = serviceID.String()
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	snapshot.Metadata = metadata
	return snapshot, nil
}

func terminalFailureChannel(eventType string) (string, bool) {
	switch strings.TrimSpace(eventType) {
	case platformwebhook.EventEmailBounced,
		platformwebhook.EventEmailRejected,
		platformwebhook.EventEmailFailed:
		return "email", true
	case platformwebhook.EventSMSUndelivered,
		platformwebhook.EventSMSFailed:
		return "sms", true
	default:
		return "", false
	}
}

func channelMessageColumn(channel string) (string, error) {
	switch strings.TrimSpace(channel) {
	case "email":
		return "email_message_id", nil
	case "sms":
		return "sms_message_id", nil
	default:
		return "", fmt.Errorf("unsupported verification delivery channel %q", channel)
	}
}
