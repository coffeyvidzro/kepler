-- name: CreateAuditEvent :one
INSERT INTO audit_events (
    team_id, actor_type, actor_user_id, actor_session_id, actor_token_id,
    action, resource_type, resource_id,
    outcome, metadata, request_id, ip_address, user_agent
) VALUES (
    sqlc.narg(team_id), sqlc.arg(actor_type), sqlc.narg(actor_user_id),
    sqlc.narg(actor_session_id), sqlc.narg(actor_token_id),
    sqlc.arg(action), sqlc.arg(resource_type), sqlc.arg(resource_id),
    sqlc.arg(outcome), sqlc.arg(metadata), sqlc.narg(request_id),
    sqlc.narg(ip_address), sqlc.narg(user_agent)
)
RETURNING *;

-- name: ListTeamAuditEvents :many
WITH cursor_event AS (
    SELECT created_at, id
    FROM audit_events
    WHERE id = sqlc.narg(before_id)
      AND team_id = sqlc.arg(team_id)
)
SELECT audit_events.*
FROM audit_events
WHERE audit_events.team_id = sqlc.arg(team_id)
  AND (
      sqlc.narg(before_id)::uuid IS NULL
      OR (audit_events.created_at, audit_events.id) < (
          SELECT cursor_event.created_at, cursor_event.id FROM cursor_event
      )
  )
ORDER BY audit_events.created_at DESC, audit_events.id DESC
LIMIT sqlc.arg(page_size);
