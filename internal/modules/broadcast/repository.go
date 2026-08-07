package broadcast

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("broadcast not found")
	ErrConflict = errors.New("broadcast conflict")
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

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
	query := `UPDATE broadcasts SET status='queued',template_version_id=$1,scheduled_at=NULL,queued_at=now(),canceled_at=NULL,revision=revision+1,updated_at=now()
		WHERE id=$2 AND team_id=$3 AND status='draft' AND deleted_at IS NULL RETURNING ` + broadcastColumns
	args := []any{versionID, id, teamID}
	if scheduledAt != nil {
		query = `UPDATE broadcasts SET status='scheduled',template_version_id=$1,scheduled_at=$2,canceled_at=NULL,revision=revision+1,updated_at=now()
			WHERE id=$3 AND team_id=$4 AND status='draft' AND deleted_at IS NULL RETURNING ` + broadcastColumns
		args = []any{versionID, scheduledAt, id, teamID}
	}
	value, err := scanBroadcast(r.db.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	return value, err
}

func (r *Repository) Cancel(ctx context.Context, teamID, id uuid.UUID) (Broadcast, error) {
	value, err := scanBroadcast(r.db.QueryRow(ctx, `UPDATE broadcasts SET status='draft',template_version_id=NULL,scheduled_at=NULL,canceled_at=now(),revision=revision+1,updated_at=now()
		WHERE id=$1 AND team_id=$2 AND status='scheduled' AND deleted_at IS NULL RETURNING `+broadcastColumns, id, teamID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Broadcast{}, ErrConflict
	}
	return value, err
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
