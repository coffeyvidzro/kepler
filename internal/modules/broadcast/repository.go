package broadcast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

var (
	ErrNotFound = errors.New("broadcast not found")
	ErrConflict = errors.New("broadcast conflict")
)

type eventEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformevent.Envelope) (platformevent.Result, error)
}

type Repository struct {
	db      *pgxpool.Pool
	queries *dbsqlc.Queries
	emitter eventEmitter
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db)}
}

func NewRepositoryWithEventEmitter(db *pgxpool.Pool, emitter eventEmitter) *Repository {
	return &Repository{db: db, queries: dbsqlc.New(db), emitter: emitter}
}

func (r *Repository) Create(ctx context.Context, teamID, segmentID uuid.UUID, topicID *uuid.UUID, templateID uuid.UUID, req CreateRequest) (Broadcast, error) {
	bindings, err := json.Marshal(req.VariableBindings)
	if err != nil {
		return Broadcast{}, fmt.Errorf("encode broadcast variable bindings: %w", err)
	}
	row, err := r.queries.CreateBroadcast(ctx, dbsqlc.CreateBroadcastParams{
		TeamID: teamID, Name: req.Name, SegmentID: segmentID, TopicID: topicID,
		TemplateID: templateID, VariableBindings: bindings,
	})
	if err != nil {
		return Broadcast{}, fmt.Errorf("create broadcast: %w", err)
	}
	return broadcastFromSQLC(row)
}

func (r *Repository) Duplicate(ctx context.Context, teamID, sourceID uuid.UUID, name string) (Broadcast, error) {
	row, err := r.queries.DuplicateBroadcast(ctx, dbsqlc.DuplicateBroadcastParams{Name: name, SourceID: sourceID, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrNotFound
	}
	if err != nil {
		return Broadcast{}, fmt.Errorf("duplicate broadcast: %w", err)
	}
	return broadcastFromSQLC(row)
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Broadcast, error) {
	rows, err := r.queries.ListBroadcasts(ctx, dbsqlc.ListBroadcastsParams{TeamID: teamID, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return nil, fmt.Errorf("list broadcasts: %w", err)
	}
	values := make([]Broadcast, 0, len(rows))
	for _, row := range rows {
		value, mapErr := broadcastFromSQLC(row)
		if mapErr != nil {
			return nil, mapErr
		}
		values = append(values, value)
	}
	return values, nil
}

func (r *Repository) Get(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	row, err := r.queries.GetBroadcast(ctx, dbsqlc.GetBroadcastParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrNotFound
	}
	if err != nil {
		return Broadcast{}, fmt.Errorf("get broadcast: %w", err)
	}
	return broadcastFromSQLC(row)
}

func (r *Repository) Update(ctx context.Context, teamID, id, segmentID uuid.UUID, topicID *uuid.UUID, templateID uuid.UUID, req UpdateRequest, current Broadcast) (Broadcast, error) {
	bindings, err := json.Marshal(current.VariableBindings)
	if err != nil {
		return Broadcast{}, fmt.Errorf("encode broadcast variable bindings: %w", err)
	}
	row, err := r.queries.UpdateBroadcastDraft(ctx, dbsqlc.UpdateBroadcastDraftParams{
		Name: current.Name, SegmentID: segmentID, TopicID: topicID, TemplateID: templateID,
		VariableBindings: bindings, ID: id, TeamID: teamID, Revision: req.Revision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	if err != nil {
		return Broadcast{}, fmt.Errorf("update broadcast: %w", err)
	}
	return broadcastFromSQLC(row)
}

func (r *Repository) Send(ctx context.Context, teamID, id, versionID uuid.UUID, scheduledAt any) (Broadcast, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Broadcast{}, fmt.Errorf("begin broadcast send: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)

	var row dbsqlc.Broadcast
	eventType := platformevent.TypeBroadcastQueued
	reason := "immediate_send"
	if scheduledAt == nil {
		row, err = queries.QueueBroadcast(ctx, dbsqlc.QueueBroadcastParams{TemplateVersionID: &versionID, ID: id, TeamID: teamID})
	} else {
		scheduled, ok := scheduledTime(scheduledAt)
		if !ok {
			return Broadcast{}, errors.New("invalid scheduled broadcast time")
		}
		row, err = queries.ScheduleBroadcast(ctx, dbsqlc.ScheduleBroadcastParams{
			TemplateVersionID: &versionID,
			ScheduledAt:       pgtype.Timestamptz{Time: scheduled, Valid: true},
			ID:                id,
			TeamID:            teamID,
		})
		eventType = platformevent.TypeBroadcastScheduled
		reason = "scheduled_send"
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	if err != nil {
		return Broadcast{}, fmt.Errorf("send broadcast: %w", err)
	}
	value, err := broadcastFromSQLC(row)
	if err != nil {
		return Broadcast{}, err
	}
	if err := emitBroadcastEvent(ctx, tx, r.emitter, eventType, value, StatusDraft, reason, nil); err != nil {
		return Broadcast{}, fmt.Errorf("emit broadcast lifecycle event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Broadcast{}, fmt.Errorf("commit broadcast send: %w", err)
	}
	return value, nil
}

func (r *Repository) QueueScheduled(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	return r.transition(ctx, platformevent.TypeBroadcastQueued, StatusScheduled, "schedule_due", nil, func(q *dbsqlc.Queries) (dbsqlc.Broadcast, error) {
		return q.QueueScheduledBroadcast(ctx, dbsqlc.QueueScheduledBroadcastParams{ID: id, TeamID: teamID})
	})
}

func (r *Repository) QueueNextDueScheduled(ctx context.Context) (Broadcast, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Broadcast{}, false, fmt.Errorf("begin due broadcast claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := r.queries.WithTx(tx).QueueNextDueBroadcast(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, false, nil
	}
	if err != nil {
		return Broadcast{}, false, fmt.Errorf("claim due broadcast: %w", err)
	}
	value, err := broadcastFromSQLC(row)
	if err != nil {
		return Broadcast{}, false, err
	}
	if err := emitBroadcastEvent(ctx, tx, r.emitter, platformevent.TypeBroadcastQueued, value, StatusScheduled, "schedule_due", nil); err != nil {
		return Broadcast{}, false, fmt.Errorf("emit due broadcast queued event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Broadcast{}, false, fmt.Errorf("commit due broadcast claim: %w", err)
	}
	return value, true, nil
}

func (r *Repository) MarkSent(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	return r.transition(ctx, platformevent.TypeBroadcastSent, StatusQueued, "recipient_fanout_completed", nil, func(q *dbsqlc.Queries) (dbsqlc.Broadcast, error) {
		return q.MarkBroadcastSent(ctx, dbsqlc.MarkBroadcastSentParams{ID: id, TeamID: teamID})
	})
}

func (r *Repository) MarkFailed(ctx context.Context, teamID, id uuid.UUID, phase, code, message string, retryable bool) (Broadcast, error) {
	failure := map[string]any{"phase": phase, "code": code, "message": message, "retryable": retryable}
	return r.transition(ctx, platformevent.TypeBroadcastFailed, StatusQueued, "orchestration_failed", failure, func(q *dbsqlc.Queries) (dbsqlc.Broadcast, error) {
		return q.MarkBroadcastFailed(ctx, dbsqlc.MarkBroadcastFailedParams{ID: id, TeamID: teamID})
	})
}

func (r *Repository) Cancel(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	return r.transition(ctx, platformevent.TypeBroadcastCanceled, StatusScheduled, "user_canceled", nil, func(q *dbsqlc.Queries) (dbsqlc.Broadcast, error) {
		return q.CancelScheduledBroadcast(ctx, dbsqlc.CancelScheduledBroadcastParams{ID: id, TeamID: teamID})
	})
}

func (r *Repository) transition(ctx context.Context, eventType platformevent.Type, from, reason string, failure map[string]any, update func(*dbsqlc.Queries) (dbsqlc.Broadcast, error)) (Broadcast, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Broadcast{}, fmt.Errorf("begin broadcast transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := update(r.queries.WithTx(tx))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	if err != nil {
		return Broadcast{}, fmt.Errorf("update broadcast transition: %w", err)
	}
	value, err := broadcastFromSQLC(row)
	if err != nil {
		return Broadcast{}, err
	}
	if err := emitBroadcastEvent(ctx, tx, r.emitter, eventType, value, from, reason, failure); err != nil {
		return Broadcast{}, fmt.Errorf("emit broadcast lifecycle event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Broadcast{}, fmt.Errorf("commit broadcast transition: %w", err)
	}
	return value, nil
}

func (r *Repository) Delete(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	row, err := r.queries.SoftDeleteBroadcast(ctx, dbsqlc.SoftDeleteBroadcastParams{ID: id, TeamID: teamID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	if err != nil {
		return Broadcast{}, fmt.Errorf("delete broadcast: %w", err)
	}
	return broadcastFromSQLC(row)
}

func (r *Repository) ListRecipients(ctx context.Context, teamID, broadcastID uuid.UUID, limit, offset int32) ([]Recipient, error) {
	rows, err := r.queries.ListBroadcastRecipients(ctx, dbsqlc.ListBroadcastRecipientsParams{
		TeamID: teamID, BroadcastID: broadcastID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list broadcast recipients: %w", err)
	}
	values := make([]Recipient, 0, len(rows))
	for _, row := range rows {
		value, mapErr := recipientFromSQLC(row)
		if mapErr != nil {
			return nil, mapErr
		}
		values = append(values, value)
	}
	return values, nil
}

type MaterializationResult struct {
	BroadcastID   uuid.UUID
	TeamID        uuid.UUID
	AudienceCount int64
	EligibleCount int64
	ExcludedCount int64
}

func (r *Repository) MaterializeNextQueuedRecipients(ctx context.Context) (MaterializationResult, bool, error) {
	if r == nil || r.db == nil || r.queries == nil {
		return MaterializationResult{}, false, errors.New("broadcast repository is not configured")
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("begin recipient materialization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)

	candidate, err := queries.ClaimNextQueuedBroadcastForMaterialization(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return MaterializationResult{}, false, nil
	}
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("claim queued broadcast: %w", err)
	}
	if err := queries.MaterializeBroadcastRecipients(ctx, dbsqlc.MaterializeBroadcastRecipientsParams{
		TopicID: candidate.TopicID, TeamID: candidate.TeamID, SegmentID: candidate.SegmentID, BroadcastID: candidate.ID,
	}); err != nil {
		return MaterializationResult{}, false, fmt.Errorf("insert broadcast recipients: %w", err)
	}
	completed, err := queries.CompleteBroadcastRecipientMaterialization(ctx, dbsqlc.CompleteBroadcastRecipientMaterializationParams{
		BroadcastID: candidate.ID, TeamID: candidate.TeamID,
	})
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("complete recipient materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return MaterializationResult{}, false, fmt.Errorf("commit recipient materialization: %w", err)
	}
	return MaterializationResult{
		BroadcastID: completed.BroadcastID,
		TeamID: completed.TeamID,
		AudienceCount: completed.AudienceCount,
		EligibleCount: completed.EligibleCount,
		ExcludedCount: completed.ExcludedCount,
	}, true, nil
}

type broadcastTransition struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

type broadcastSummary struct {
	AudienceCount   int64 `json:"audience_count"`
	EligibleCount   int64 `json:"eligible_count"`
	SuppressedCount int64 `json:"suppressed_count"`
	QueuedCount     int64 `json:"queued_count"`
	FailedCount     int64 `json:"failed_count"`
}

func emitBroadcastEvent(ctx context.Context, tx pgx.Tx, emitter eventEmitter, eventType platformevent.Type, value Broadcast, from, reason string, failure map[string]any) error {
	if emitter == nil {
		emitter = platformevent.DefaultEmitter()
	}
	if emitter == nil {
		return nil
	}
	teamID, err := uuid.Parse(value.TeamID)
	if err != nil {
		return fmt.Errorf("parse broadcast team id: %w", err)
	}
	objectID, err := uuid.Parse(value.ID)
	if err != nil {
		return fmt.Errorf("parse broadcast id: %w", err)
	}
	payload := map[string]any{
		"broadcast": value,
		"transition": broadcastTransition{From: from, To: value.Status, Reason: reason},
		"summary": broadcastSummary{
			AudienceCount: value.AudienceCount, EligibleCount: value.EligibleCount,
			SuppressedCount: value.SuppressedCount, QueuedCount: value.QueuedCount,
			FailedCount: value.FailedCount,
		},
	}
	if failure != nil {
		payload["failure"] = failure
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode broadcast event: %w", err)
	}
	_, err = emitter.EmitTx(ctx, tx, platformevent.Envelope{
		Type: eventType, TeamID: teamID, ObjectType: "broadcast", ObjectID: &objectID, Data: data,
	})
	return err
}

func broadcastFromSQLC(row dbsqlc.Broadcast) (Broadcast, error) {
	bindings := map[string]any{}
	if len(row.VariableBindings) > 0 {
		if err := json.Unmarshal(row.VariableBindings, &bindings); err != nil {
			return Broadcast{}, fmt.Errorf("decode broadcast variable bindings: %w", err)
		}
	}
	return Broadcast{
		ID: row.ID.String(), TeamID: row.TeamID.String(), Name: row.Name, Status: row.Status,
		SegmentID: row.SegmentID.String(), TopicID: uuidStringPointer(row.TopicID),
		TemplateID: row.TemplateID.String(), TemplateVersionID: uuidStringPointer(row.TemplateVersionID),
		VariableBindings: bindings, ScheduledAt: timestampPointer(row.ScheduledAt),
		QueuedAt: timestampPointer(row.QueuedAt), SentAt: timestampPointer(row.SentAt),
		CanceledAt: timestampPointer(row.CanceledAt), AudienceCount: row.AudienceCount,
		EligibleCount: row.EligibleCount, SuppressedCount: row.SuppressedCount,
		QueuedCount: row.QueuedCount, FailedCount: row.FailedCount, Revision: row.Revision,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func recipientFromSQLC(row dbsqlc.BroadcastRecipient) (Recipient, error) {
	snapshot := map[string]any{}
	if len(row.ContactSnapshot) > 0 {
		if err := json.Unmarshal(row.ContactSnapshot, &snapshot); err != nil {
			return Recipient{}, fmt.Errorf("decode broadcast recipient snapshot: %w", err)
		}
	}
	return Recipient{
		ID: row.ID.String(), BroadcastID: row.BroadcastID.String(), ContactID: uuidStringPointer(row.ContactID),
		Email: row.Email, FirstName: row.FirstName, LastName: row.LastName, ContactSnapshot: snapshot,
		Status: row.Status, ExclusionReason: row.ExclusionReason, EmailMessageID: uuidStringPointer(row.EmailMessageID),
		CreatedAt: row.CreatedAt.Time, QueuedAt: timestampPointer(row.QueuedAt),
	}, nil
}

func uuidStringPointer(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func scheduledTime(value any) (time.Time, bool) {
	switch scheduled := value.(type) {
	case time.Time:
		return scheduled, true
	case *time.Time:
		if scheduled != nil {
			return *scheduled, true
		}
	}
	return time.Time{}, false
}
