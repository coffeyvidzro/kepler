-- name: CreateWebhookDeliveryAttempt :one
INSERT INTO webhook_delivery_attempts (
    delivery_id,
    attempt_number,
    outcome,
    request_timestamp,
    started_at,
    completed_at,
    duration_ms,
    response_status,
    response_headers,
    response_body,
    error_message
)
SELECT
    delivery.id,
    sqlc.arg(attempt_number),
    sqlc.arg(outcome),
    sqlc.arg(request_timestamp),
    sqlc.arg(started_at),
    sqlc.arg(completed_at),
    sqlc.arg(duration_ms),
    sqlc.narg(response_status),
    sqlc.arg(response_headers),
    sqlc.narg(response_body),
    sqlc.narg(error_message)
FROM webhook_deliveries AS delivery
JOIN webhook_events AS event ON event.id = delivery.event_id
WHERE delivery.id = sqlc.arg(delivery_id)
  AND event.team_id = sqlc.arg(team_id)
RETURNING webhook_delivery_attempts.*;

-- name: GetWebhookDeliveryAttempt :one
SELECT attempt.*
FROM webhook_delivery_attempts AS attempt
JOIN webhook_deliveries AS delivery ON delivery.id = attempt.delivery_id
JOIN webhook_events AS event ON event.id = delivery.event_id
JOIN teams AS team ON team.id = event.team_id
WHERE attempt.id = sqlc.arg(id)
  AND event.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: ListWebhookDeliveryAttempts :many
SELECT attempt.*
FROM webhook_delivery_attempts AS attempt
JOIN webhook_deliveries AS delivery ON delivery.id = attempt.delivery_id
JOIN webhook_events AS event ON event.id = delivery.event_id
JOIN teams AS team ON team.id = event.team_id
WHERE attempt.delivery_id = sqlc.arg(delivery_id)
  AND event.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY attempt.attempt_number DESC, attempt.created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);
