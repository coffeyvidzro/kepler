package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	messagingrouting "github.com/coffeyvidzro/dugble/server/internal/delivery/messaging/routing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging"
	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/messaging/routing"
	"github.com/coffeyvidzro/dugble/server/internal/platform/messaging/sender"
	smsapi "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	"github.com/coffeyvidzro/dugble/server/pkg/pgconv"
)

var ErrMessageNotFound = errors.New("sms message not found")
var ErrMessageNotSchedulable = errors.New("sms message is not a pending scheduled message")

type Repository struct {
	db      *pgxpool.Pool
	dbtx    dbsqlc.DBTX
	queries *dbsqlc.Queries
	tx      pgx.Tx
	emitter webhookEmitter
}

type webhookEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformwebhook.Event) (uuid.UUID, int64, error)
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, dbtx: db, queries: dbsqlc.New(db)}
}

func NewRepositoryWithWebhookEmitter(db *pgxpool.Pool, emitter webhookEmitter) *Repository {
	repository := NewRepository(db)
	repository.emitter = emitter
	return repository
}

func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin sms transaction: %w", err)
	}
	return tx, nil
}

func (r *Repository) WithTx(tx pgx.Tx) *Repository {
	return &Repository{db: r.db, dbtx: tx, queries: r.queries.WithTx(tx), tx: tx, emitter: r.emitter}
}

type createMessageParams struct {
	TeamID             uuid.UUID
	SenderID           *uuid.UUID
	To                 string
	From               string
	Body               string
	Status             string
	Segments           int32
	Metadata           json.RawMessage
	Tags               []Tag
	ScheduledAt        *time.Time
	DestinationCountry string
}

func (r *Repository) Create(ctx context.Context, params createMessageParams) (Message, error) {
	tags, err := json.Marshal(params.Tags)
	if err != nil {
		return Message{}, fmt.Errorf("encode SMS tags: %w", err)
	}
	row, err := r.queries.CreateSMSMessage(ctx, dbsqlc.CreateSMSMessageParams{
		TeamID:                  params.TeamID,
		SenderProviderBindingID: params.SenderID,
		ToNumber:                params.To,
		FromName:                params.From,
		Body:                    params.Body,
		Status:                  params.Status,
		Segments:                params.Segments,
		Metadata:                ensureMetadata(params.Metadata),
		Tags:                    tags,
		ScheduledAt:             pgconv.NullableTimestamptz(params.ScheduledAt),
		DestinationCountry:      params.DestinationCountry,
	})
	if err != nil {
		return Message{}, fmt.Errorf("create sms message: %w", err)
	}
	return messageFromSQLC(row), nil
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Message, error) {
	rows, err := r.queries.ListSMSMessages(ctx, dbsqlc.ListSMSMessagesParams{TeamID: teamID, LimitCount: limit, OffsetCount: offset})
	if err != nil {
		return nil, fmt.Errorf("list sms messages: %w", err)
	}
	messages := make([]Message, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, messageFromSQLC(row))
	}
	return messages, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (Message, error) {
	row, err := r.queries.GetSMSMessage(ctx, dbsqlc.GetSMSMessageParams{ID: id, TeamID: teamID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrMessageNotFound
		}
		return Message{}, fmt.Errorf("get sms message: %w", err)
	}
	return messageFromSQLC(row), nil
}

func (r *Repository) CancelTx(ctx context.Context, tx pgx.Tx, id, teamID uuid.UUID) (Message, error) {
	if err := lockScheduledSMS(ctx, tx, id, teamID); err != nil {
		return Message{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE sms_messages SET status = $3, updated_at = now() WHERE id = $1 AND team_id = $2`, id, teamID, StatusCanceled); err != nil {
		return Message{}, fmt.Errorf("cancel SMS message: %w", err)
	}
	return r.WithTx(tx).Get(ctx, id, teamID)
}

func (r *Repository) RescheduleTx(ctx context.Context, tx pgx.Tx, id, teamID uuid.UUID, scheduledAt time.Time) (Message, error) {
	if err := lockScheduledSMS(ctx, tx, id, teamID); err != nil {
		return Message{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE sms_messages SET scheduled_at = $3, updated_at = now() WHERE id = $1 AND team_id = $2`, id, teamID, scheduledAt); err != nil {
		return Message{}, fmt.Errorf("reschedule SMS message: %w", err)
	}
	return r.WithTx(tx).Get(ctx, id, teamID)
}

func lockScheduledSMS(ctx context.Context, tx pgx.Tx, id, teamID uuid.UUID) error {
	var status string
	var scheduledAt *time.Time
	err := tx.QueryRow(ctx, `SELECT status, scheduled_at FROM sms_messages WHERE id = $1 AND team_id = $2 FOR UPDATE`, id, teamID).Scan(&status, &scheduledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMessageNotFound
	}
	if err != nil {
		return fmt.Errorf("lock scheduled SMS message: %w", err)
	}
	if status != StatusQueued || scheduledAt == nil || !scheduledAt.After(time.Now().UTC().Add(scheduleMutationCutoff)) {
		return ErrMessageNotSchedulable
	}
	return nil
}

func (r *Repository) MarkProcessing(ctx context.Context, id uuid.UUID, teamID uuid.UUID) (Message, error) {
	row, err := r.queries.MarkSMSMessageProcessing(ctx, dbsqlc.MarkSMSMessageProcessingParams{ID: id, TeamID: teamID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrMessageNotFound
		}
		return Message{}, fmt.Errorf("mark sms message processing: %w", err)
	}
	return messageFromSQLC(row), nil
}

func (r *Repository) MarkDeliveryUnknown(ctx context.Context, id uuid.UUID, teamID uuid.UUID, message string) (Message, error) {
	row, err := r.queries.MarkSMSMessageDeliveryUnknown(ctx, dbsqlc.MarkSMSMessageDeliveryUnknownParams{
		ID: id, TeamID: teamID, ErrorMessage: &message,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Message{}, ErrMessageNotFound
		}
		return Message{}, fmt.Errorf("mark sms message delivery unknown: %w", err)
	}
	return messageFromSQLC(row), nil
}

func (r *Repository) MarkSubmitted(ctx context.Context, id uuid.UUID, teamID uuid.UUID, providerID string, providerMessageID string, status string) (Message, error) {
	if r.tx == nil && r.emitter != nil {
		return withSMSLifecycleTx(ctx, r, func(repository *Repository) (Message, error) {
			return repository.MarkSubmitted(ctx, id, teamID, providerID, providerMessageID, status)
		})
	}
	row, err := r.queries.MarkSMSMessageSubmitted(ctx, dbsqlc.MarkSMSMessageSubmittedParams{
		ID:                id,
		TeamID:            teamID,
		ProviderID:        &providerID,
		ProviderMessageID: &providerMessageID,
		Status:            status,
	})
	if err != nil {
		return Message{}, fmt.Errorf("mark sms message submitted: %w", err)
	}
	message := messageFromSQLC(row)
	if err := r.emitLifecycle(ctx, message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func (r *Repository) MarkFailed(ctx context.Context, id uuid.UUID, teamID uuid.UUID, message string) (Message, error) {
	if r.tx == nil && r.emitter != nil {
		return withSMSLifecycleTx(ctx, r, func(repository *Repository) (Message, error) {
			return repository.MarkFailed(ctx, id, teamID, message)
		})
	}
	row, err := r.queries.MarkSMSMessageFailed(ctx, dbsqlc.MarkSMSMessageFailedParams{ID: id, TeamID: teamID, ErrorMessage: &message})
	if err != nil {
		return Message{}, fmt.Errorf("mark sms message failed: %w", err)
	}
	updated := messageFromSQLC(row)
	if err := r.emitLifecycle(ctx, updated); err != nil {
		return Message{}, err
	}
	return updated, nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, teamID uuid.UUID, status string) (Message, error) {
	if r.tx == nil && r.emitter != nil {
		return withSMSLifecycleTx(ctx, r, func(repository *Repository) (Message, error) {
			return repository.UpdateStatus(ctx, id, teamID, status)
		})
	}
	row, err := r.queries.UpdateSMSMessageStatus(ctx, dbsqlc.UpdateSMSMessageStatusParams{ID: id, TeamID: teamID, Status: status})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// A concurrent status update may have already advanced or completed the
			// message. Return that authoritative state without emitting a duplicate
			// lifecycle event for the rejected transition.
			return r.Get(ctx, id, teamID)
		}
		return Message{}, fmt.Errorf("update sms message status: %w", err)
	}
	message := messageFromSQLC(row)
	if err := r.emitLifecycle(ctx, message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func withSMSLifecycleTx(ctx context.Context, repository *Repository, operation func(*Repository) (Message, error)) (Message, error) {
	return postgres.InTransactionResult(ctx, repository.db, func(tx pgx.Tx) (Message, error) {
		return operation(repository.WithTx(tx))
	})
}

func (r *Repository) emitLifecycle(ctx context.Context, message Message) error {
	if r.emitter == nil {
		return nil
	}
	if r.tx == nil {
		return errors.New("SMS lifecycle webhook transaction is required")
	}
	event, ok, err := smsLifecycleEvent(message)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if _, _, err := r.emitter.EmitTx(ctx, r.tx, event); err != nil {
		return fmt.Errorf("emit SMS lifecycle webhook: %w", err)
	}
	return nil
}

func smsLifecycleEvent(message Message) (platformwebhook.Event, bool, error) {
	eventTypes := map[string]string{
		StatusSubmitted:   platformwebhook.EventSMSSubmitted,
		StatusSent:        platformwebhook.EventSMSSent,
		StatusDelivered:   platformwebhook.EventSMSDelivered,
		StatusUndelivered: platformwebhook.EventSMSUndelivered,
		StatusFailed:      platformwebhook.EventSMSFailed,
	}
	eventType, ok := eventTypes[message.Status]
	if !ok {
		return platformwebhook.Event{}, false, nil
	}
	messageID, err := uuid.Parse(message.ID)
	if err != nil {
		return platformwebhook.Event{}, false, fmt.Errorf("parse SMS message id for webhook: %w", err)
	}
	teamID, err := uuid.Parse(message.TeamID)
	if err != nil {
		return platformwebhook.Event{}, false, fmt.Errorf("parse SMS team id for webhook: %w", err)
	}
	payload, err := json.Marshal(message.Response())
	if err != nil {
		return platformwebhook.Event{}, false, fmt.Errorf("encode SMS webhook payload: %w", err)
	}
	return platformwebhook.Event{
		ID: uuid.New(), TeamID: teamID, Type: eventType, ObjectType: "sms", ObjectID: &messageID,
		Payload: payload, OccurredAt: message.UpdatedAt,
	}, true, nil
}

func (r *Repository) FindApprovedSender(ctx context.Context, teamID uuid.UUID, name string) (*uuid.UUID, error) {
	id, err := r.queries.FindApprovedSMSSender(ctx, dbsqlc.FindApprovedSMSSenderParams{TeamID: teamID, Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find approved sender id: %w", err)
	}
	return &id, nil
}

func messageFromSQLC(row dbsqlc.SmsMessage) Message {
	message := Message{
		ID:                 row.ID.String(),
		TeamID:             row.TeamID.String(),
		To:                 row.ToNumber,
		From:               row.FromName,
		Body:               row.Body,
		Status:             row.Status,
		ProviderID:         row.ProviderID,
		ProviderMessageID:  row.ProviderMessageID,
		Segments:           row.Segments,
		ErrorMessage:       row.ErrorMessage,
		Metadata:           ensureMetadata(row.Metadata),
		Tags:               decodeTags(row.Tags),
		ScheduledAt:        pgconv.TimestamptzToTimePtr(row.ScheduledAt),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
		DestinationCountry: row.DestinationCountry,
	}
	if row.SenderProviderBindingID != nil {
		value := row.SenderProviderBindingID.String()
		message.SenderID = &value
	}
	if row.SubmittedAt.Valid {
		message.SubmittedAt = &row.SubmittedAt.Time
	}
	if row.DeliveredAt.Valid {
		message.DeliveredAt = &row.DeliveredAt.Time
	}
	return message
}

func decodeTags(value []byte) []Tag {
	result := []Tag{}
	_ = json.Unmarshal(value, &result)
	return result
}

func ensureMetadata(metadata json.RawMessage) json.RawMessage {
	if len(metadata) == 0 {
		return json.RawMessage(`{}`)
	}
	return metadata
}

func (r *Repository) ResolveDeliveryRoutes(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
) ([]platformrouting.Route, error) {
	var status string
	var country string
	var assetID uuid.NullUUID
	err := r.dbtx.QueryRow(ctx, `
		SELECT message.status, message.destination_country, binding.sender_asset_id
		FROM sms_messages AS message
		LEFT JOIN sender_provider_bindings AS binding
		  ON binding.id = message.sender_provider_binding_id
		WHERE message.id = $1 AND message.team_id = $2
	`, id, teamID).Scan(&status, &country, &assetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load SMS route request: %w", err)
	}
	if status != StatusProcessing {
		return nil, ErrMessageNotFound
	}
	request := platformrouting.Request{
		TeamID:             teamID,
		Channel:            messaging.ChannelSMS,
		DestinationCountry: country,
		RequiredCapabilities: []sender.Capability{
			sender.CapabilitySenderIDRegistration,
		},
	}
	if assetID.Valid {
		value := assetID.UUID
		request.SenderAssetID = &value
	}
	routes, err := messagingrouting.ResolveAll(ctx, r.dbtx, request)
	if err != nil {
		return nil, fmt.Errorf("resolve SMS delivery routes: %w", err)
	}
	return routes, nil
}

func (r *Repository) CreateDeliveryAttempt(
	ctx context.Context,
	id uuid.UUID,
	teamID uuid.UUID,
	route platformrouting.Route,
) (uuid.UUID, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin SMS delivery attempt: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var country string
	var currentAssetID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT message.destination_country, binding.sender_asset_id
		FROM sms_messages AS message
		JOIN sender_provider_bindings AS binding
		  ON binding.id = message.sender_provider_binding_id
		WHERE message.id = $1 AND message.team_id = $2 AND message.status = 'processing'
		FOR UPDATE OF message
	`, id, teamID).Scan(&country, &currentAssetID); errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrMessageNotFound
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("lock SMS message for attempt: %w", err)
	}
	if currentAssetID != route.SenderAssetID {
		return uuid.Nil, platformrouting.ErrNoEligibleRoute
	}

	var eligible bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM sender_provider_bindings AS binding
			JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
			JOIN sender_asset_grants AS grant_record
			  ON grant_record.sender_asset_id = asset.id
			 AND grant_record.team_id = $1
			 AND grant_record.channel = 'sms'
			 AND grant_record.status = 'active'
			 AND grant_record.revoked_at IS NULL
			WHERE binding.id = $2
			  AND asset.id = $3
			  AND asset.channel = 'sms'
			  AND asset.status = 'active'
			  AND asset.health_status <> 'degraded'
			  AND binding.status = 'active'
			  AND binding.verified
			  AND binding.disabled_at IS NULL
			  AND binding.health_status <> 'degraded'
			  AND lower(binding.provider) = lower($4)
			  AND binding.provider_account = $5
			  AND COALESCE(binding.region, '') = $6
			  AND COALESCE(binding.country_code::text, '') = $7
			  AND (binding.country_code IS NULL OR binding.country_code = $8)
		)
	`, teamID, route.SenderProviderBindingID, route.SenderAssetID, route.Provider,
		route.ProviderAccount, route.Region, route.CountryCode, country).Scan(&eligible); err != nil {
		return uuid.Nil, fmt.Errorf("verify SMS delivery route: %w", err)
	}
	if !eligible {
		return uuid.Nil, platformrouting.ErrNoEligibleRoute
	}

	var attemptNumber int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(attempt_number), 0) + 1
		FROM message_delivery_attempts
		WHERE sms_message_id = $1
	`, id).Scan(&attemptNumber); err != nil {
		return uuid.Nil, fmt.Errorf("calculate SMS delivery attempt number: %w", err)
	}
	attemptID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO message_delivery_attempts (
			id, team_id, channel, sms_message_id, attempt_number, status,
			provider, provider_account, sender_asset_id, sender_provider_binding_id
		)
		VALUES ($1, $2, 'sms', $3, $4, 'claimed', $5, $6, $7, $8)
	`, attemptID, teamID, id, attemptNumber, route.Provider, route.ProviderAccount,
		route.SenderAssetID, route.SenderProviderBindingID); err != nil {
		return uuid.Nil, fmt.Errorf("create SMS delivery attempt: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sms_messages
		SET sender_provider_binding_id = $3, updated_at = now()
		WHERE id = $1 AND team_id = $2 AND status = 'processing'
	`, id, teamID, route.SenderProviderBindingID); err != nil {
		return uuid.Nil, fmt.Errorf("persist SMS delivery route: %w", err)
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
		return ErrMessageNotFound
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
	if _, err := r.WithTx(tx).MarkDeliveryUnknown(ctx, id, teamID, deliveryErrorText(cause)); err != nil {
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
	if _, err := r.WithTx(tx).MarkFailed(ctx, id, teamID, deliveryErrorText(cause)); err != nil {
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
		return ErrMessageNotFound
	}
	if _, err := r.WithTx(tx).MarkSubmitted(
		ctx,
		id,
		teamID,
		response.ProviderID,
		response.ProviderMsgID,
		MapProviderStatus(response.Status),
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
		if _, updateErr := r.WithTx(tx).MarkDeliveryUnknown(
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
		if _, err := r.WithTx(tx).MarkFailed(
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
		if _, err := r.WithTx(tx).MarkDeliveryUnknown(
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
		return ErrMessageNotFound
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
