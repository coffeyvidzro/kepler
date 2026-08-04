package expiry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

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

type Repository struct {
	db     *pgxpool.Pool
	events *platformevent.Emitter
}

func NewRepository(db *pgxpool.Pool, events *platformevent.Emitter) *Repository {
	return &Repository{db: db, events: events}
}

func (repository *Repository) ExpireBatch(ctx context.Context, batchSize int32) (int, error) {
	if repository == nil || repository.db == nil || repository.events == nil {
		return 0, errors.New("verification expiry repository is not configured")
	}
	if batchSize <= 0 {
		return 0, errors.New("verification expiry batch size must be positive")
	}

	tx, err := repository.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, fmt.Errorf("begin verification expiry transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, team_id, service_id, channel, recipient, status, locale, metadata,
		       attempt_count, resend_count, expires_at, approved_at, expired_at,
		       canceled_at, failed_at, created_at, updated_at
		FROM verifications
		WHERE status = 'pending'
		  AND expires_at <= now()
		ORDER BY expires_at, created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, batchSize)
	if err != nil {
		return 0, fmt.Errorf("claim expired verifications: %w", err)
	}

	snapshots := make([]VerificationSnapshot, 0, batchSize)
	for rows.Next() {
		var snapshot VerificationSnapshot
		var id, teamID, serviceID uuid.UUID
		var metadata []byte
		if err := rows.Scan(
			&id, &teamID, &serviceID, &snapshot.Channel, &snapshot.Recipient,
			&snapshot.Status, &snapshot.Locale, &metadata, &snapshot.AttemptCount,
			&snapshot.ResendCount, &snapshot.ExpiresAt, &snapshot.ApprovedAt,
			&snapshot.ExpiredAt, &snapshot.CanceledAt, &snapshot.FailedAt,
			&snapshot.CreatedAt, &snapshot.UpdatedAt,
		); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan expired verification: %w", err)
		}
		snapshot.ID = id.String()
		snapshot.TeamID = teamID.String()
		snapshot.ServiceID = serviceID.String()
		if len(metadata) == 0 {
			metadata = []byte(`{}`)
		}
		snapshot.Metadata = metadata
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate expired verifications: %w", err)
	}
	rows.Close()

	for index := range snapshots {
		snapshot := &snapshots[index]
		verificationID, err := uuid.Parse(snapshot.ID)
		if err != nil {
			return 0, fmt.Errorf("parse verification id: %w", err)
		}
		teamID, err := uuid.Parse(snapshot.TeamID)
		if err != nil {
			return 0, fmt.Errorf("parse verification team id: %w", err)
		}

		var expiredAt, updatedAt time.Time
		err = tx.QueryRow(ctx, `
			UPDATE verifications
			SET status = 'expired', expired_at = now(), updated_at = now()
			WHERE id = $1 AND team_id = $2 AND status = 'pending' AND expires_at <= now()
			RETURNING expired_at, updated_at
		`, verificationID, teamID).Scan(&expiredAt, &updatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("expire verification %s: %w", verificationID, err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE verification_challenges
			SET status = 'expired', updated_at = now()
			WHERE verification_id = $1
			  AND team_id = $2
			  AND status IN ('queued', 'dispatching', 'dispatched')
		`, verificationID, teamID); err != nil {
			return 0, fmt.Errorf("expire verification challenges %s: %w", verificationID, err)
		}

		snapshot.Status = "expired"
		snapshot.ExpiredAt = &expiredAt
		snapshot.UpdatedAt = updatedAt
		data, err := json.Marshal(snapshot)
		if err != nil {
			return 0, fmt.Errorf("encode verification expiry event: %w", err)
		}
		if _, err := repository.events.EmitTx(ctx, tx, platformevent.Envelope{
			Type: platformevent.TypeVerificationExpired, TeamID: teamID,
			ObjectType: "verification", ObjectID: &verificationID,
			Data: data, OccurredAt: updatedAt,
		}); err != nil {
			return 0, fmt.Errorf("emit verification expiry event: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit verification expiry batch: %w", err)
	}
	return len(snapshots), nil
}
