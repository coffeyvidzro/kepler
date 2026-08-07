-- name: CreateBroadcast :one
INSERT INTO broadcasts (
    team_id, name, segment_id, topic_id, template_id, variable_bindings
) VALUES (
    sqlc.arg(team_id), sqlc.arg(name), sqlc.arg(segment_id), sqlc.narg(topic_id),
    sqlc.arg(template_id), sqlc.arg(variable_bindings)
)
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

-- name: CancelScheduledBroadcast :one
UPDATE broadcasts
SET status = 'draft',
    template_version_id = NULL,
    scheduled_at = NULL,
    canceled_at = now(),
    revision = revision + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'scheduled'
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
