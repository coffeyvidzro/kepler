-- name: CreateWebhookEvent :one
INSERT INTO webhook_events (
    id,
    team_id,
    event_type,
    object_type,
    object_id,
    payload,
    occurred_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(team_id),
    sqlc.arg(event_type),
    sqlc.arg(object_type),
    sqlc.narg(object_id),
    sqlc.arg(payload),
    sqlc.arg(occurred_at)
)
ON CONFLICT (id) DO UPDATE
SET id = EXCLUDED.id
RETURNING *;

-- name: GetWebhookEvent :one
SELECT event.*
FROM webhook_events AS event
JOIN teams AS team ON team.id = event.team_id
WHERE event.id = sqlc.arg(id)
  AND event.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: ListWebhookEvents :many
SELECT event.*
FROM webhook_events AS event
JOIN teams AS team ON team.id = event.team_id
WHERE event.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY event.created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: ListWebhookEventsForObject :many
SELECT event.*
FROM webhook_events AS event
JOIN teams AS team ON team.id = event.team_id
WHERE event.team_id = sqlc.arg(team_id)
  AND event.object_type = sqlc.arg(object_type)
  AND event.object_id IS NOT DISTINCT FROM sqlc.narg(object_id)
  AND team.status = 'active'
ORDER BY event.occurred_at DESC, event.created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: ListWebhookEventsFiltered :many
SELECT event.*
FROM webhook_events AS event
JOIN teams AS team ON team.id = event.team_id
WHERE event.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
  AND (sqlc.narg(event_type)::text IS NULL OR event.event_type = sqlc.narg(event_type))
  AND (sqlc.narg(object_type)::text IS NULL OR event.object_type = sqlc.narg(object_type))
  AND (sqlc.narg(object_id)::uuid IS NULL OR event.object_id = sqlc.narg(object_id))
  AND (sqlc.narg(before_occurred_at)::timestamptz IS NULL OR event.occurred_at < sqlc.narg(before_occurred_at))
ORDER BY event.occurred_at DESC, event.id DESC
LIMIT sqlc.arg(limit_count);
