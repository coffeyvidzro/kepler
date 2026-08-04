-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (
    team_id,
    url,
    signing_secret,
    enabled,
    subscribed_events
)
SELECT
    team.id,
    sqlc.arg(url),
    sqlc.arg(signing_secret),
    true,
    sqlc.arg(subscribed_events)
FROM teams AS team
WHERE team.id = sqlc.arg(team_id)
  AND team.status = 'active'
RETURNING *;

-- name: ListWebhookEndpoints :many
SELECT endpoint.*
FROM webhook_endpoints AS endpoint
JOIN teams AS team ON team.id = endpoint.team_id
WHERE endpoint.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY endpoint.created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: GetWebhookEndpoint :one
SELECT endpoint.*
FROM webhook_endpoints AS endpoint
JOIN teams AS team ON team.id = endpoint.team_id
WHERE endpoint.id = sqlc.arg(id)
  AND endpoint.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: UpdateWebhookEndpoint :one
UPDATE webhook_endpoints AS endpoint
SET url = sqlc.arg(url),
    enabled = sqlc.arg(enabled),
    subscribed_events = sqlc.arg(subscribed_events),
    disabled_at = CASE
        WHEN sqlc.arg(enabled)::boolean THEN NULL
        ELSE COALESCE(endpoint.disabled_at, now())
    END,
    consecutive_failures = CASE WHEN sqlc.arg(enabled)::boolean THEN 0 ELSE endpoint.consecutive_failures END,
    last_failure_at = CASE WHEN sqlc.arg(enabled)::boolean THEN NULL ELSE endpoint.last_failure_at END,
    disabled_reason = CASE WHEN sqlc.arg(enabled)::boolean THEN NULL ELSE COALESCE(endpoint.disabled_reason, 'manual') END,
    updated_at = now()
FROM teams AS team
WHERE endpoint.id = sqlc.arg(id)
  AND endpoint.team_id = sqlc.arg(team_id)
  AND team.id = endpoint.team_id
  AND team.status = 'active'
RETURNING endpoint.*;

-- name: DisableWebhookEndpoint :one
UPDATE webhook_endpoints AS endpoint
SET enabled = false,
    disabled_at = COALESCE(endpoint.disabled_at, now()),
    disabled_reason = 'manual',
    updated_at = now()
FROM teams AS team
WHERE endpoint.id = sqlc.arg(id)
  AND endpoint.team_id = sqlc.arg(team_id)
  AND team.id = endpoint.team_id
  AND team.status = 'active'
RETURNING endpoint.*;

-- name: RotateWebhookEndpointSecret :one
UPDATE webhook_endpoints AS endpoint
SET signing_secret = sqlc.arg(signing_secret),
    updated_at = now()
FROM teams AS team
WHERE endpoint.id = sqlc.arg(id)
  AND endpoint.team_id = sqlc.arg(team_id)
  AND team.id = endpoint.team_id
  AND team.status = 'active'
RETURNING endpoint.*;

-- name: ListSubscribedWebhookEndpoints :many
SELECT endpoint.*
FROM webhook_endpoints AS endpoint
JOIN teams AS team ON team.id = endpoint.team_id
WHERE endpoint.team_id = sqlc.arg(team_id)
  AND endpoint.enabled = true
  AND endpoint.disabled_at IS NULL
  AND sqlc.arg(event_type)::text = ANY(endpoint.subscribed_events)
  AND team.status = 'active'
ORDER BY endpoint.created_at, endpoint.id;
