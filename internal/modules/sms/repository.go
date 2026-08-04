package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/coffeyvidzro/dugble/server/internal/database"
	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
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
		TeamID:             params.TeamID,
		SenderID:           params.SenderID,
		ToNumber:           params.To,
		FromName:           params.From,
		Body:               params.Body,
		Status:             params.Status,
		Segments:           params.Segments,
		Metadata:           ensureMetadata(params.Metadata),
		Tags:               tags,
		ScheduledAt:        pgconv.NullableTimestamptz(params.ScheduledAt),
		DestinationCountry: params.DestinationCountry,
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
	return database.InTransactionResult(ctx, repository.db, func(tx pgx.Tx) (Message, error) {
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
	if row.SenderID != nil {
		value := row.SenderID.String()
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
