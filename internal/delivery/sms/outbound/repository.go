package smsdelivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

var ErrNoEligibleRoute = errors.New("no eligible SMS delivery route")

// DeliveryRoute is the provider route for one approved Sender ID.
type DeliveryRoute struct {
	SenderID uuid.UUID
	Provider string
}

type Repository struct {
	db       *pgxpool.Pool
	messages *smsmodule.Repository
}

func NewRepository(db *pgxpool.Pool, messages *smsmodule.Repository) *Repository {
	return &Repository{db: db, messages: messages}
}

func (r *Repository) MarkProcessing(ctx context.Context, id, teamID uuid.UUID) (smsmodule.Message, error) {
	return r.messages.MarkProcessing(ctx, id, teamID)
}

func (r *Repository) Get(ctx context.Context, id, teamID uuid.UUID) (smsmodule.Message, error) {
	return r.messages.Get(ctx, id, teamID)
}

func (r *Repository) MarkDeliveryUnknown(
	ctx context.Context,
	id, teamID uuid.UUID,
	message string,
) (smsmodule.Message, error) {
	return r.messages.MarkDeliveryUnknown(ctx, id, teamID, message)
}

func (r *Repository) MarkFailed(
	ctx context.Context,
	id, teamID uuid.UUID,
	message string,
) (smsmodule.Message, error) {
	return r.messages.MarkFailed(ctx, id, teamID, message)
}

func (r *Repository) ResolveDeliveryRoute(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
) (DeliveryRoute, error) {
	var status, country string
	var senderID uuid.NullUUID
	err := r.db.QueryRow(ctx, `
		SELECT message.status, message.destination_country, message.sender_id
		FROM sms_messages AS message
		WHERE message.id = $1 AND message.team_id = $2
	`, id, teamID).Scan(&status, &country, &senderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliveryRoute{}, smsmodule.ErrMessageNotFound
	}
	if err != nil {
		return DeliveryRoute{}, fmt.Errorf("load SMS route request: %w", err)
	}
	if status != smsmodule.StatusProcessing {
		return DeliveryRoute{}, smsmodule.ErrMessageNotFound
	}
	if !senderID.Valid {
		return DeliveryRoute{}, ErrNoEligibleRoute
	}
	var route DeliveryRoute
	err = r.db.QueryRow(ctx, `
		SELECT sender_id.id, sender_id.provider
		FROM sender_ids AS sender_id
		WHERE sender_id.id = $1
		  AND sender_id.team_id = $2
		  AND sender_id.country_code = $3
		  AND sender_id.status = 'approved'
		  AND sender_id.provider_whitelisted
		  AND sender_id.provider IS NOT NULL
		  AND sender_id.disabled_at IS NULL
		  AND sender_id.health_status <> 'degraded'
	`, senderID.UUID, teamID, country).Scan(&route.SenderID, &route.Provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeliveryRoute{}, ErrNoEligibleRoute
	}
	if err != nil {
		return DeliveryRoute{}, fmt.Errorf("resolve SMS delivery route: %w", err)
	}
	return route, nil
}

func (r *Repository) CreateDeliveryAttempt(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	route DeliveryRoute,
) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin SMS delivery attempt: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var country string
	var currentSenderID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT message.destination_country, message.sender_id
		FROM sms_messages AS message
		WHERE message.id = $1 AND message.team_id = $2 AND message.status = 'processing'
		FOR UPDATE OF message
	`, id, teamID).Scan(&country, &currentSenderID); errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, smsmodule.ErrMessageNotFound
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("lock SMS message for attempt: %w", err)
	}
	if currentSenderID != route.SenderID {
		return uuid.Nil, ErrNoEligibleRoute
	}

	var eligible bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sender_ids AS sender_id
			WHERE sender_id.id = $1 AND sender_id.team_id = $2
			  AND sender_id.country_code = $3
			  AND sender_id.status = 'approved'
			  AND sender_id.provider_whitelisted
			  AND sender_id.disabled_at IS NULL
			  AND sender_id.health_status <> 'degraded'
			  AND lower(sender_id.provider) = lower($4)
		)
	`, route.SenderID, teamID, country, route.Provider).Scan(&eligible); err != nil {
		return uuid.Nil, fmt.Errorf("verify SMS delivery route: %w", err)
	}
	if !eligible {
		return uuid.Nil, ErrNoEligibleRoute
	}

	var attemptNumber int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_number), 0) + 1
		FROM message_delivery_attempts WHERE sms_message_id = $1
	`, id).Scan(&attemptNumber); err != nil {
		return uuid.Nil, fmt.Errorf("calculate SMS delivery attempt number: %w", err)
	}
	attemptID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO message_delivery_attempts (
			id, team_id, channel, sms_message_id, attempt_number, status,
			provider, sender_id
		) VALUES ($1, $2, 'sms', $3, $4, 'claimed', $5, $6)
	`, attemptID, teamID, id, attemptNumber, route.Provider, route.SenderID); err != nil {
		return uuid.Nil, fmt.Errorf("create SMS delivery attempt: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit SMS delivery attempt: %w", err)
	}
	return attemptID, nil
}

func (r *Repository) MarkDeliveryAttemptStarted(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	attemptID uuid.UUID,
) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE message_delivery_attempts
		SET status = 'request_started', request_started_at = now(), updated_at = now()
		WHERE id = $3 AND sms_message_id = $1 AND team_id = $2
		  AND channel = 'sms' AND status = 'claimed'
	`, id, teamID, attemptID)
	if err != nil {
		return fmt.Errorf("mark SMS provider request started: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return smsmodule.ErrMessageNotFound
	}
	return nil
}

func (r *Repository) MarkDeliveryAttemptRetryable(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	attemptID uuid.UUID,
	cause error,
) error {
	return completeSMSAttemptTx(ctx, r.db, id, teamID, attemptID, "retryable_failure", cause)
}

func (r *Repository) MarkDeliveryAttemptUnknown(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	attemptID uuid.UUID,
	cause error,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin unknown SMS attempt completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := completeSMSAttemptTx(ctx, tx, id, teamID, attemptID, "submission_unknown", cause); err != nil {
		return err
	}
	if _, err := r.messages.WithTx(tx).MarkDeliveryUnknown(ctx, id, teamID, deliveryErrorText(cause)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) MarkDeliveryAttemptFailed(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	attemptID uuid.UUID,
	cause error,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin failed SMS attempt completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := completeSMSAttemptTx(ctx, tx, id, teamID, attemptID, "permanent_failure", cause); err != nil {
		return err
	}
	if _, err := r.messages.WithTx(tx).MarkFailed(ctx, id, teamID, deliveryErrorText(cause)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) MarkDeliveryAttemptSubmitted(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	attemptID uuid.UUID,
	response *smsapi.SendResponse,
) error {
	if response == nil {
		return errors.New("SMS provider response is required")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin submitted SMS attempt completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE message_delivery_attempts
		SET status = 'submitted', provider = $4, provider_message_id = $5,
			provider_status = $6, request_completed_at = now(),
			submitted_at = COALESCE(submitted_at, now()),
			error_code = NULL, error_message = NULL, updated_at = now()
		WHERE id = $3 AND sms_message_id = $1 AND team_id = $2
		  AND channel = 'sms' AND status = 'request_started'
	`, id, teamID, attemptID, response.ProviderID, response.ProviderMsgID, response.Status)
	if err != nil {
		return fmt.Errorf("mark SMS attempt submitted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return smsmodule.ErrMessageNotFound
	}
	if _, err := r.messages.WithTx(tx).MarkSubmitted(
		ctx,
		id,
		teamID,
		response.ProviderID,
		response.ProviderMsgID,
		smsmodule.MapProviderStatus(response.Status),
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) FinalizeInFlightDelivery(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	cause error,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin in-flight SMS finalization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var attemptID uuid.UUID
	var attemptStatus string
	err = tx.QueryRow(ctx, `
		SELECT attempt.id, attempt.status
		FROM message_delivery_attempts AS attempt
		JOIN sms_messages AS message
		  ON message.id = attempt.sms_message_id
		 AND message.team_id = attempt.team_id
		WHERE message.id = $1
		  AND message.team_id = $2
		  AND message.status = 'processing'
		  AND attempt.channel = 'sms'
		  AND attempt.status IN ('claimed', 'request_started')
		ORDER BY attempt.attempt_number DESC
		LIMIT 1
		FOR UPDATE OF message, attempt
	`, id, teamID).Scan(&attemptID, &attemptStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, updateErr := r.messages.WithTx(tx).MarkDeliveryUnknown(
			ctx,
			id,
			teamID,
			deliveryErrorText(cause),
		); updateErr != nil {
			return updateErr
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return fmt.Errorf("lock in-flight SMS attempt: %w", err)
	}

	if attemptStatus == "claimed" {
		if err := completeSMSAttemptTx(
			ctx,
			tx,
			id,
			teamID,
			attemptID,
			"permanent_failure",
			cause,
		); err != nil {
			return err
		}
		if _, err := r.messages.WithTx(tx).MarkFailed(
			ctx,
			id,
			teamID,
			deliveryErrorText(cause),
		); err != nil {
			return err
		}
	} else {
		if err := completeSMSAttemptTx(
			ctx,
			tx,
			id,
			teamID,
			attemptID,
			"submission_unknown",
			cause,
		); err != nil {
			return err
		}
		if _, err := r.messages.WithTx(tx).MarkDeliveryUnknown(
			ctx,
			id,
			teamID,
			deliveryErrorText(cause),
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type smsAttemptExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func completeSMSAttemptTx(
	ctx context.Context,
	db smsAttemptExecer,
	id uuid.UUID,
	teamID uuid.UUID,
	attemptID uuid.UUID,
	status string,
	cause error,
) error {
	tag, err := db.Exec(ctx, `
		UPDATE message_delivery_attempts
		SET status = $4, error_code = $4, error_message = $5,
			request_completed_at = COALESCE(request_completed_at, now()),
			terminal_at = CASE
				WHEN $4 IN ('retryable_failure', 'permanent_failure')
				THEN COALESCE(terminal_at, now())
				ELSE terminal_at
			END,
			updated_at = now()
		WHERE id = $3 AND sms_message_id = $1 AND team_id = $2
		  AND channel = 'sms' AND status IN ('claimed', 'request_started')
	`, id, teamID, attemptID, status, deliveryErrorText(cause))
	if err != nil {
		return fmt.Errorf("complete SMS delivery attempt: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return smsmodule.ErrMessageNotFound
	}
	return nil
}

func deliveryErrorText(err error) string {
	if err == nil {
		return "unknown SMS delivery failure"
	}
	const maxLength = 2000
	value := err.Error()
	if len(value) > maxLength {
		return value[:maxLength]
	}
	return value
}

type messageRepository interface {
	MarkProcessing(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (smsmodule.Message, error)
	MarkDeliveryUnknown(context.Context, uuid.UUID, uuid.UUID, string) (smsmodule.Message, error)
	MarkFailed(context.Context, uuid.UUID, uuid.UUID, string) (smsmodule.Message, error)
	ResolveDeliveryRoute(context.Context, uuid.UUID, uuid.UUID) (DeliveryRoute, error)
	CreateDeliveryAttempt(context.Context, uuid.UUID, uuid.UUID, DeliveryRoute) (uuid.UUID, error)
	MarkDeliveryAttemptStarted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	MarkDeliveryAttemptRetryable(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error
	MarkDeliveryAttemptUnknown(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error
	MarkDeliveryAttemptFailed(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, error) error
	MarkDeliveryAttemptSubmitted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, *smsapi.SendResponse) error
	FinalizeInFlightDelivery(context.Context, uuid.UUID, uuid.UUID, error) error
}

type providerSender interface {
	SendWithProvider(context.Context, string, smsapi.SendRequest) (*smsapi.SendResponse, error)
	ProviderIDs() []string
}
