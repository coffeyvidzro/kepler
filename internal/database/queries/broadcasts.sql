-- name: CreateBroadcast :one
INSERT INTO broadcasts (
    team_id, name, segment_id, topic_id, template_id, variable_bindings
) VALUES (
    sqlc.arg(team_id), sqlc.arg(name), sqlc.arg(segment_id), sqlc.narg(topic_id),
    sqlc.arg(template_id), sqlc.arg(variable_bindings)
)
RETURNING *;

-- name: DuplicateBroadcast :one
INSERT INTO broadcasts (
    team_id, name, segment_id, topic_id, template_id, variable_bindings
)
SELECT
    source.team_id,
    sqlc.arg(name),
    source.segment_id,
    source.topic_id,
    source.template_id,
    source.variable_bindings
FROM broadcasts AS source
WHERE source.id = sqlc.arg(source_id)
  AND source.team_id = sqlc.arg(team_id)
  AND source.deleted_at IS NULL
RETURNING *;

-- name: ListBroadcasts :many
SELECT *
FROM broadcasts
WHERE team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetBroadcast :one
SELECT *
FROM broadcasts
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL;

-- name: UpdateBroadcastDraft :one
UPDATE broadcasts
SET name = sqlc.arg(name),
    segment_id = sqlc.arg(segment_id),
    topic_id = sqlc.narg(topic_id),
    template_id = sqlc.arg(template_id),
    variable_bindings = sqlc.arg(variable_bindings),
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'draft'
  AND revision = sqlc.arg(revision)
  AND deleted_at IS NULL
RETURNING *;

-- name: ScheduleBroadcast :one
UPDATE broadcasts
SET status = 'scheduled',
    template_version_id = sqlc.arg(template_version_id),
    scheduled_at = sqlc.arg(scheduled_at),
    canceled_at = NULL,
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'draft'
  AND deleted_at IS NULL
RETURNING *;

-- name: QueueBroadcast :one
UPDATE broadcasts
SET status = 'queued',
    template_version_id = sqlc.arg(template_version_id),
    scheduled_at = NULL,
    queued_at = now(),
    canceled_at = NULL,
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'draft'
  AND deleted_at IS NULL
RETURNING *;

-- name: QueueScheduledBroadcast :one
UPDATE broadcasts
SET status = 'queued',
    queued_at = now(),
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'scheduled'
  AND deleted_at IS NULL
RETURNING *;

-- name: QueueNextDueBroadcast :one
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
RETURNING broadcast.*;

-- name: CancelScheduledBroadcast :one
UPDATE broadcasts
SET status = 'canceled',
    scheduled_at = NULL,
    canceled_at = now(),
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'scheduled'
  AND deleted_at IS NULL
RETURNING *;

-- name: MarkBroadcastSent :one
UPDATE broadcasts
SET status = 'sent',
    sent_at = now(),
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'queued'
  AND deleted_at IS NULL
RETURNING *;

-- name: MarkBroadcastFailed :one
UPDATE broadcasts
SET status = 'failed',
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'queued'
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteBroadcast :one
UPDATE broadcasts
SET deleted_at = now(), updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status IN ('draft','canceled')
  AND deleted_at IS NULL
RETURNING *;

-- name: ClaimNextQueuedBroadcastForMaterialization :one
SELECT id, team_id, segment_id, topic_id
FROM broadcasts
WHERE status = 'queued'
  AND recipients_materialized_at IS NULL
  AND deleted_at IS NULL
ORDER BY queued_at, id
FOR UPDATE SKIP LOCKED
LIMIT 1;

-- name: CompleteBroadcastRecipientMaterialization :one
WITH counts AS (
    SELECT
        count(*) AS audience_count,
        count(*) FILTER (WHERE status = 'pending') AS eligible_count,
        count(*) FILTER (WHERE status = 'excluded') AS excluded_count
    FROM broadcast_recipients
    WHERE broadcast_id = sqlc.arg(broadcast_id)
      AND team_id = sqlc.arg(team_id)
)
UPDATE broadcasts AS broadcast
SET audience_count = counts.audience_count,
    eligible_count = counts.eligible_count,
    suppressed_count = counts.excluded_count,
    failed_count = 0,
    recipients_materialized_at = now(),
    revision = broadcast.revision + 1,
    updated_at = now()
FROM counts
WHERE broadcast.id = sqlc.arg(broadcast_id)
  AND broadcast.team_id = sqlc.arg(team_id)
RETURNING broadcast.id AS broadcast_id,
          broadcast.team_id,
          counts.audience_count,
          counts.eligible_count,
          counts.excluded_count;
