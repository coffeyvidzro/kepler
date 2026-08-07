package broadcast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
)

var (
	ErrNotFound = errors.New("broadcast not found")
	ErrConflict = errors.New("broadcast conflict")
)

type Repository struct {
	db      *pgxpool.Pool
	emitter eventEmitter
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func NewRepositoryWithEventEmitter(db *pgxpool.Pool, emitter eventEmitter) *Repository {
	return &Repository{db: db, emitter: emitter}
}

const broadcastColumns = `id,team_id,name,status,segment_id,topic_id,template_id,template_version_id,
variable_bindings,scheduled_at,queued_at,sent_at,canceled_at,audience_count,eligible_count,
suppressed_count,queued_count,failed_count,revision,created_at,updated_at`

func scanBroadcast(row pgx.Row) (Broadcast, error) {
	var value Broadcast
	var bindings []byte
	err := row.Scan(&value.ID, &value.TeamID, &value.Name, &value.Status, &value.SegmentID,
		&value.TopicID, &value.TemplateID, &value.TemplateVersionID, &bindings,
		&value.ScheduledAt, &value.QueuedAt, &value.SentAt, &value.CanceledAt,
		&value.AudienceCount, &value.EligibleCount, &value.SuppressedCount,
		&value.QueuedCount, &value.FailedCount, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	if err != nil {
		return Broadcast{}, err
	}
	if err := json.Unmarshal(bindings, &value.VariableBindings); err != nil {
		return Broadcast{}, err
	}
	if value.VariableBindings == nil {
		value.VariableBindings = map[string]any{}
	}
	return value, nil
}

func (r *Repository) Create(ctx context.Context, teamID, segmentID uuid.UUID, topicID *uuid.UUID, templateID uuid.UUID, req CreateRequest) (Broadcast, error) {
	bindings, err := json.Marshal(req.VariableBindings)
	if err != nil {
		return Broadcast{}, err
	}
	return scanBroadcast(r.db.QueryRow(ctx, `INSERT INTO broadcasts (team_id,name,segment_id,topic_id,template_id,variable_bindings)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+broadcastColumns,
		teamID, req.Name, segmentID, topicID, templateID, bindings))
}

func (r *Repository) Duplicate(ctx context.Context, teamID, sourceID uuid.UUID, name string) (Broadcast, error) {
	value, err := scanBroadcast(r.db.QueryRow(ctx, `INSERT INTO broadcasts (team_id,name,segment_id,topic_id,template_id,variable_bindings)
		SELECT team_id,$1,segment_id,topic_id,template_id,variable_bindings
		FROM broadcasts
		WHERE id=$2 AND team_id=$3 AND deleted_at IS NULL
		RETURNING `+broadcastColumns, name, sourceID, teamID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrNotFound
	}
	return value, err
}

func (r *Repository) List(ctx context.Context, teamID uuid.UUID, limit, offset int32) ([]Broadcast, error) {
	rows, err := r.db.Query(ctx, `SELECT `+broadcastColumns+` FROM broadcasts WHERE team_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Broadcast{}
	for rows.Next() {
		value, err := scanBroadcast(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *Repository) Get(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	value, err := scanBroadcast(r.db.QueryRow(ctx, `SELECT `+broadcastColumns+` FROM broadcasts WHERE id=$1 AND team_id=$2 AND deleted_at IS NULL`, id, teamID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrNotFound
	}
	return value, err
}

func (r *Repository) Update(ctx context.Context, teamID, id, segmentID uuid.UUID, topicID *uuid.UUID, templateID uuid.UUID, req UpdateRequest, current Broadcast) (Broadcast, error) {
	bindings, err := json.Marshal(current.VariableBindings)
	if err != nil {
		return Broadcast{}, err
	}
	value, err := scanBroadcast(r.db.QueryRow(ctx, `UPDATE broadcasts SET name=$1,segment_id=$2,topic_id=$3,template_id=$4,variable_bindings=$5,revision=revision+1,updated_at=now()
		WHERE id=$6 AND team_id=$7 AND status='draft' AND revision=$8 AND deleted_at IS NULL RETURNING `+broadcastColumns,
		current.Name, segmentID, topicID, templateID, bindings, id, teamID, req.Revision))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	return value, err
}

func (r *Repository) Send(ctx context.Context, teamID, id, versionID uuid.UUID, scheduledAt any) (Broadcast, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Broadcast{}, fmt.Errorf("begin broadcast send: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	query := `UPDATE broadcasts SET status='queued',template_version_id=$1,scheduled_at=NULL,queued_at=now(),canceled_at=NULL,revision=revision+1,updated_at=now()
		WHERE id=$2 AND team_id=$3 AND status='draft' AND deleted_at IS NULL RETURNING ` + broadcastColumns
	args := []any{versionID, id, teamID}
	eventType := platformevent.TypeBroadcastQueued
	reason := "immediate_send"
	if scheduledAt != nil {
		query = `UPDATE broadcasts SET status='scheduled',template_version_id=$1,scheduled_at=$2,canceled_at=NULL,revision=revision+1,updated_at=now()
			WHERE id=$3 AND team_id=$4 AND status='draft' AND deleted_at IS NULL RETURNING ` + broadcastColumns
		args = []any{versionID, scheduledAt, id, teamID}
		eventType = platformevent.TypeBroadcastScheduled
		reason = "scheduled_send"
	}
	value, err := scanBroadcast(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
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
	return r.transition(ctx, `UPDATE broadcasts SET status='queued',queued_at=now(),revision=revision+1,updated_at=now()
		WHERE id=$1 AND team_id=$2 AND status='scheduled' AND deleted_at IS NULL RETURNING `+broadcastColumns,
		[]any{id, teamID}, platformevent.TypeBroadcastQueued, StatusScheduled, "schedule_due", nil)
}

// QueueNextDueScheduled atomically claims the next due scheduled Broadcast,
// moves it to queued, and emits broadcast.queued in the same transaction.
// FOR UPDATE SKIP LOCKED lets multiple workers poll safely without durable
// lease state; a crashed worker rolls back and releases the row automatically.
func (r *Repository) QueueNextDueScheduled(ctx context.Context) (Broadcast, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Broadcast{}, false, fmt.Errorf("begin due broadcast claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	value, err := scanBroadcast(tx.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM broadcasts
			WHERE status = 'scheduled'
			  AND scheduled_at <= now()
			  AND deleted_at IS NULL
			ORDER BY scheduled_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE broadcasts AS broadcast
		SET status = 'queued',
			queued_at = now(),
			revision = broadcast.revision + 1,
			updated_at = now()
		FROM candidate
		WHERE broadcast.id = candidate.id
		RETURNING `+broadcastColumns))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, false, nil
	}
	if err != nil {
		return Broadcast{}, false, fmt.Errorf("claim due broadcast: %w", err)
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
	return r.transition(ctx, `UPDATE broadcasts SET status='sent',sent_at=now(),revision=revision+1,updated_at=now()
		WHERE id=$1 AND team_id=$2 AND status='queued' AND deleted_at IS NULL RETURNING `+broadcastColumns,
		[]any{id, teamID}, platformevent.TypeBroadcastSent, StatusQueued, "recipient_fanout_completed", nil)
}

func (r *Repository) MarkFailed(ctx context.Context, teamID, id uuid.UUID, phase, code, message string, retryable bool) (Broadcast, error) {
	failure := map[string]any{"phase": phase, "code": code, "message": message, "retryable": retryable}
	return r.transition(ctx, `UPDATE broadcasts SET status='failed',revision=revision+1,updated_at=now()
		WHERE id=$1 AND team_id=$2 AND status='queued' AND deleted_at IS NULL RETURNING `+broadcastColumns,
		[]any{id, teamID}, platformevent.TypeBroadcastFailed, StatusQueued, "orchestration_failed", failure)
}

func (r *Repository) Cancel(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	return r.transition(ctx, `UPDATE broadcasts SET status='canceled',scheduled_at=NULL,canceled_at=now(),revision=revision+1,updated_at=now()
		WHERE id=$1 AND team_id=$2 AND status='scheduled' AND deleted_at IS NULL RETURNING `+broadcastColumns,
		[]any{id, teamID}, platformevent.TypeBroadcastCanceled, StatusScheduled, "user_canceled", nil)
}

func (r *Repository) transition(ctx context.Context, query string, args []any, eventType platformevent.Type, from, reason string, failure map[string]any) (Broadcast, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Broadcast{}, fmt.Errorf("begin broadcast transition: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value, err := scanBroadcast(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
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
	value, err := scanBroadcast(r.db.QueryRow(ctx, `UPDATE broadcasts SET deleted_at=now(),updated_at=now() WHERE id=$1 AND team_id=$2 AND status IN ('draft','canceled') AND deleted_at IS NULL RETURNING `+broadcastColumns, id, teamID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	return value, err
}

func (r *Repository) ListRecipients(ctx context.Context, teamID, broadcastID uuid.UUID, limit, offset int32) ([]Recipient, error) {
	rows, err := r.db.Query(ctx, `SELECT id,broadcast_id,contact_id,email,first_name,last_name,contact_snapshot,status,exclusion_reason,email_message_id,created_at,queued_at
		FROM broadcast_recipients WHERE team_id=$1 AND broadcast_id=$2 ORDER BY created_at,id LIMIT $3 OFFSET $4`, teamID, broadcastID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Recipient{}
	for rows.Next() {
		var value Recipient
		var snapshot []byte
		if err := rows.Scan(&value.ID, &value.BroadcastID, &value.ContactID, &value.Email, &value.FirstName, &value.LastName, &snapshot, &value.Status, &value.ExclusionReason, &value.EmailMessageID, &value.CreatedAt, &value.QueuedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(snapshot, &value.ContactSnapshot); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

type eventEmitter interface {
	EmitTx(context.Context, pgx.Tx, platformevent.Envelope) (platformevent.Result, error)
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
		"broadcast":  value,
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
		Type:       eventType,
		TeamID:     teamID,
		ObjectType: "broadcast",
		ObjectID:   &objectID,
		Data:       data,
	})
	return err
}

type MaterializationResult struct {
	BroadcastID   uuid.UUID
	TeamID        uuid.UUID
	AudienceCount int64
	EligibleCount int64
	ExcludedCount int64
}

// MaterializeNextQueuedRecipients freezes the next queued Broadcast audience.
// The Broadcast row is locked until recipients, counters, and the completion
// marker are committed, making the operation safe to retry across workers.
func (r *Repository) MaterializeNextQueuedRecipients(ctx context.Context) (MaterializationResult, bool, error) {
	if r == nil || r.db == nil {
		return MaterializationResult{}, false, errors.New("broadcast repository is not configured")
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("begin recipient materialization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var broadcastID, teamID, segmentID uuid.UUID
	var topicID *uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id, team_id, segment_id, topic_id
		FROM broadcasts
		WHERE status = 'queued'
		  AND recipients_materialized_at IS NULL
		  AND deleted_at IS NULL
		ORDER BY queued_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&broadcastID, &teamID, &segmentID, &topicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MaterializationResult{}, false, nil
	}
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("claim queued broadcast: %w", err)
	}

	_, err = tx.Exec(ctx, `
		WITH candidates AS (
			SELECT
				c.id AS contact_id,
				c.email,
				lower(btrim(c.email)) AS normalized_email,
				c.first_name,
				c.last_name,
				jsonb_build_object(
					'id', c.id,
					'email', c.email,
					'first_name', c.first_name,
					'last_name', c.last_name,
					'properties', COALESCE(properties.values, '{}'::jsonb)
				) AS contact_snapshot,
				CASE
					WHEN c.email !~* '^[A-Z0-9.!#$%&''*+/=?^_{|}~-]+@[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?(?:\.[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?)+$' THEN 'invalid_email'
					WHEN c.unsubscribed THEN 'global_unsubscribe'
					WHEN EXISTS (
						SELECT 1 FROM suppressions s
						WHERE s.team_id = c.team_id AND lower(s.email) = lower(c.email)
					) THEN 'suppressed'
					WHEN $4::uuid IS NOT NULL AND COALESCE(cts.subscription, t.default_subscription) = 'opt_out' THEN 'topic_unsubscribed'
					ELSE NULL
				END AS exclusion_reason
			FROM contact_segments cs
			JOIN contacts c
			  ON c.id = cs.contact_id AND c.team_id = cs.team_id
			LEFT JOIN topics t
			  ON t.id = $4 AND t.team_id = c.team_id
			LEFT JOIN contact_topic_subscriptions cts
			  ON cts.contact_id = c.id AND cts.topic_id = $4 AND cts.team_id = c.team_id
			LEFT JOIN LATERAL (
				SELECT jsonb_object_agg(
					cp.key,
					CASE cpv.value_type
						WHEN 'string' THEN to_jsonb(cpv.string_value)
						WHEN 'number' THEN to_jsonb(cpv.number_value)
					END
				) AS values
				FROM contact_property_values cpv
				JOIN contact_properties cp
				  ON cp.id = cpv.contact_property_id AND cp.team_id = cpv.team_id
				WHERE cpv.contact_id = c.id AND cpv.team_id = c.team_id
			) properties ON true
			WHERE cs.team_id = $2 AND cs.segment_id = $3
		)
		INSERT INTO broadcast_recipients (
			team_id, broadcast_id, contact_id, email, normalized_email,
			first_name, last_name, contact_snapshot, status, exclusion_reason
		)
		SELECT
			$2, $1, contact_id, email, normalized_email,
			first_name, last_name, contact_snapshot,
			CASE WHEN exclusion_reason IS NULL THEN 'pending' ELSE 'excluded' END,
			exclusion_reason
		FROM candidates
		ON CONFLICT DO NOTHING
	`, broadcastID, teamID, segmentID, topicID)
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("insert broadcast recipients: %w", err)
	}

	var result MaterializationResult
	result.BroadcastID = broadcastID
	result.TeamID = teamID
	err = tx.QueryRow(ctx, `
		WITH counts AS (
			SELECT
				count(*) AS audience_count,
				count(*) FILTER (WHERE status = 'pending') AS eligible_count,
				count(*) FILTER (WHERE status = 'excluded') AS excluded_count
			FROM broadcast_recipients
			WHERE broadcast_id = $1 AND team_id = $2
		)
		UPDATE broadcasts AS b
		SET audience_count = counts.audience_count,
			eligible_count = counts.eligible_count,
			suppressed_count = counts.excluded_count,
			failed_count = 0,
			recipients_materialized_at = now(),
			revision = revision + 1,
			updated_at = now()
		FROM counts
		WHERE b.id = $1 AND b.team_id = $2
		RETURNING counts.audience_count, counts.eligible_count, counts.excluded_count
	`, broadcastID, teamID).Scan(&result.AudienceCount, &result.EligibleCount, &result.ExcludedCount)
	if err != nil {
		return MaterializationResult{}, false, fmt.Errorf("complete recipient materialization: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return MaterializationResult{}, false, fmt.Errorf("commit recipient materialization: %w", err)
	}
	return result, true, nil
}
