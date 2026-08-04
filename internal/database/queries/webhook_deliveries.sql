-- name: CreateWebhookDelivery :one
INSERT INTO webhook_deliveries (
    event_id,
    endpoint_id,
    status,
    next_attempt_at
)
SELECT
    event.id,
    endpoint.id,
    'pending',
    sqlc.arg(next_attempt_at)
FROM webhook_events AS event
JOIN webhook_endpoints AS endpoint
  ON endpoint.id = sqlc.arg(endpoint_id)
 AND endpoint.team_id = event.team_id
JOIN teams AS team ON team.id = event.team_id
WHERE event.id = sqlc.arg(event_id)
  AND endpoint.enabled = true
  AND endpoint.disabled_at IS NULL
  AND team.status = 'active'
ON CONFLICT (event_id, endpoint_id) DO UPDATE
SET event_id = EXCLUDED.event_id
RETURNING *;

-- name: CreateWebhookDeliveriesForEvent :execrows
INSERT INTO webhook_deliveries (event_id, endpoint_id, status, next_attempt_at)
SELECT
    sqlc.arg(event_id),
    endpoint.id,
    'pending',
    sqlc.arg(next_attempt_at)
FROM webhook_endpoints AS endpoint
JOIN webhook_events AS event
  ON event.id = sqlc.arg(event_id)
 AND event.team_id = endpoint.team_id
JOIN teams AS team ON team.id = event.team_id
WHERE endpoint.enabled = true
  AND endpoint.disabled_at IS NULL
  AND event.event_type = ANY(endpoint.subscribed_events)
  AND team.status = 'active'
ON CONFLICT (event_id, endpoint_id) DO NOTHING;

-- name: GetWebhookDelivery :one
SELECT delivery.*
FROM webhook_deliveries AS delivery
JOIN webhook_events AS event ON event.id = delivery.event_id
JOIN teams AS team ON team.id = event.team_id
WHERE delivery.id = sqlc.arg(id)
  AND event.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: ListWebhookDeliveriesForEvent :many
SELECT delivery.*
FROM webhook_deliveries AS delivery
JOIN webhook_events AS event ON event.id = delivery.event_id
JOIN teams AS team ON team.id = event.team_id
WHERE delivery.event_id = sqlc.arg(event_id)
  AND event.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY delivery.created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: CancelWebhookDeliveriesForEndpoint :execrows
UPDATE webhook_deliveries
SET status = 'canceled',
    last_error = 'Webhook endpoint disabled before delivery',
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
WHERE endpoint_id = sqlc.arg(endpoint_id)
  AND status IN ('pending', 'retrying');

-- name: ClaimWebhookDeliveries :many
WITH candidates AS (
    SELECT delivery.id
    FROM webhook_deliveries AS delivery
    JOIN webhook_endpoints AS endpoint ON endpoint.id = delivery.endpoint_id
    JOIN webhook_events AS event ON event.id = delivery.event_id
    JOIN teams AS team ON team.id = event.team_id
    WHERE delivery.status IN ('pending', 'retrying')
      AND delivery.next_attempt_at <= now()
      AND endpoint.enabled = true
      AND endpoint.disabled_at IS NULL
      AND team.status = 'active'
      AND (
          delivery.locked_at IS NULL
          OR delivery.locked_at < sqlc.arg(stale_before)
      )
    ORDER BY delivery.next_attempt_at, delivery.created_at
    FOR UPDATE OF delivery SKIP LOCKED
    LIMIT sqlc.arg(limit_count)
)
UPDATE webhook_deliveries AS delivery
SET locked_at = now(),
    locked_by = sqlc.arg(worker_id),
    attempt_count = delivery.attempt_count + 1,
    last_attempt_at = now(),
    updated_at = now()
FROM candidates,
     webhook_events AS event,
     webhook_endpoints AS endpoint
WHERE delivery.id = candidates.id
  AND event.id = delivery.event_id
  AND endpoint.id = delivery.endpoint_id
RETURNING
    delivery.id,
    delivery.event_id,
    delivery.endpoint_id,
    delivery.attempt_count,
    event.team_id,
    event.event_type,
    event.payload,
    event.occurred_at,
    endpoint.url,
    endpoint.signing_secret;

-- name: MarkWebhookDeliverySucceeded :one
WITH succeeded AS (
UPDATE webhook_deliveries AS delivery
SET status = 'succeeded',
    response_status = sqlc.arg(response_status),
    response_body = sqlc.narg(response_body),
    last_error = NULL,
    delivered_at = now(),
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
WHERE delivery.id = sqlc.arg(id)
  AND delivery.locked_by = sqlc.arg(worker_id)
RETURNING delivery.*
), reset_endpoint AS (
UPDATE webhook_endpoints
SET consecutive_failures = 0,
    last_failure_at = NULL,
    disabled_reason = NULL,
    updated_at = now()
WHERE id = (SELECT endpoint_id FROM succeeded)
  AND enabled = true
  AND consecutive_failures > 0
RETURNING id
)
SELECT * FROM succeeded;

-- name: ScheduleWebhookDeliveryRetry :one
UPDATE webhook_deliveries AS delivery
SET status = 'retrying',
    next_attempt_at = sqlc.arg(next_attempt_at),
    response_status = sqlc.narg(response_status),
    response_body = sqlc.narg(response_body),
    last_error = sqlc.arg(last_error),
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
WHERE delivery.id = sqlc.arg(id)
  AND delivery.locked_by = sqlc.arg(worker_id)
RETURNING delivery.*;

-- name: MarkWebhookDeliveryFailed :one
WITH failed AS (
UPDATE webhook_deliveries AS delivery
SET status = 'failed',
    response_status = sqlc.narg(response_status),
    response_body = sqlc.narg(response_body),
    last_error = sqlc.arg(last_error),
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
WHERE delivery.id = sqlc.arg(id)
  AND delivery.locked_by = sqlc.arg(worker_id)
RETURNING delivery.*
), update_endpoint AS (
UPDATE webhook_endpoints
SET consecutive_failures = consecutive_failures + 1,
    last_failure_at = now(),
    enabled = CASE
        WHEN consecutive_failures + 1 >= sqlc.arg(auto_disable_after)::integer THEN false
        ELSE enabled
    END,
    disabled_at = CASE
        WHEN consecutive_failures + 1 >= sqlc.arg(auto_disable_after)::integer THEN COALESCE(disabled_at, now())
        ELSE disabled_at
    END,
    disabled_reason = CASE
        WHEN enabled AND consecutive_failures + 1 >= sqlc.arg(auto_disable_after)::integer THEN 'failure_threshold'
        ELSE disabled_reason
    END,
    updated_at = now()
WHERE id = (SELECT endpoint_id FROM failed)
RETURNING id
)
SELECT * FROM failed;

-- name: ReleaseWebhookDeliveryClaim :execrows
UPDATE webhook_deliveries
SET locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND locked_by = sqlc.arg(worker_id);

-- name: RetryWebhookDelivery :one
UPDATE webhook_deliveries AS delivery
SET status = 'pending',
    next_attempt_at = now(),
    response_status = NULL,
    response_body = NULL,
    last_error = NULL,
    delivered_at = NULL,
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
FROM webhook_events AS event,
     webhook_endpoints AS endpoint,
     teams AS team
WHERE delivery.id = sqlc.arg(id)
  AND event.id = delivery.event_id
  AND endpoint.id = delivery.endpoint_id
  AND team.id = event.team_id
  AND event.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
  AND endpoint.enabled = true
  AND endpoint.disabled_at IS NULL
  AND delivery.status = 'failed'
RETURNING delivery.*;

-- name: ReplayWebhookDelivery :one
UPDATE webhook_deliveries AS delivery
SET status = 'pending',
    replay_count = delivery.replay_count + 1,
    last_replayed_at = now(),
    next_attempt_at = now(),
    response_status = NULL,
    response_body = NULL,
    last_error = NULL,
    delivered_at = NULL,
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
FROM webhook_events AS event,
     webhook_endpoints AS endpoint,
     teams AS team
WHERE delivery.id = sqlc.arg(id)
  AND event.id = delivery.event_id
  AND endpoint.id = delivery.endpoint_id
  AND team.id = event.team_id
  AND event.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
  AND endpoint.enabled = true
  AND endpoint.disabled_at IS NULL
  AND delivery.status IN ('succeeded', 'failed', 'canceled')
RETURNING delivery.*;
