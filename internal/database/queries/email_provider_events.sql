-- name: CreateEmailProviderEvent :one
INSERT INTO email_provider_events (
    email_message_id,
    provider,
    transport,
    provider_notification_id,
    provider_message_id,
    event_type,
    occurred_at,
    received_at,
    normalized_payload,
    provider_payload,
    next_attempt_at
) VALUES (
    sqlc.narg(email_message_id),
    sqlc.arg(provider),
    sqlc.arg(transport),
    sqlc.arg(provider_notification_id),
    sqlc.arg(provider_message_id),
    sqlc.arg(event_type),
    sqlc.arg(occurred_at),
    sqlc.arg(received_at),
    sqlc.arg(normalized_payload),
    sqlc.arg(provider_payload),
    sqlc.narg(next_attempt_at)
)
ON CONFLICT (provider, transport, provider_notification_id) DO NOTHING
RETURNING *;

-- name: GetEmailProviderEventForUpdate :one
SELECT *
FROM email_provider_events
WHERE id = sqlc.arg(id)
  AND provider = sqlc.arg(provider)
  AND transport = sqlc.arg(transport)
FOR UPDATE;

-- name: LinkEmailProviderEvent :execrows
UPDATE email_provider_events
SET email_message_id = sqlc.arg(email_message_id)
WHERE id = sqlc.arg(id)
  AND email_message_id IS NULL;

-- name: ClaimEmailProviderEvent :one
UPDATE email_provider_events
SET attempt_count = attempt_count + 1,
    last_attempt_at = now(),
    next_attempt_at = now() + (sqlc.arg(lease_seconds)::bigint * interval '1 second')
WHERE id = sqlc.arg(id)
  AND processed_at IS NULL
  AND dead_lettered_at IS NULL
  AND (next_attempt_at IS NULL OR next_attempt_at <= now())
RETURNING id, attempt_count;

-- name: ClaimDueEmailProviderEvents :many
WITH due AS (
    SELECT id
    FROM email_provider_events
    WHERE processed_at IS NULL
      AND dead_lettered_at IS NULL
      AND next_attempt_at IS NOT NULL
      AND next_attempt_at <= now()
    ORDER BY next_attempt_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
)
UPDATE email_provider_events AS event
SET attempt_count = event.attempt_count + 1,
    last_attempt_at = now(),
    next_attempt_at = now() + (sqlc.arg(lease_seconds)::bigint * interval '1 second')
FROM due
WHERE event.id = due.id
RETURNING event.id, event.attempt_count;

-- name: MarkEmailProviderEventProcessed :execrows
UPDATE email_provider_events
SET processed_at = COALESCE(processed_at, now()),
    next_attempt_at = NULL,
    last_error = NULL
WHERE id = sqlc.arg(id);

-- name: RescheduleEmailProviderEvent :execrows
UPDATE email_provider_events
SET next_attempt_at = now() + (sqlc.arg(delay_seconds)::bigint * interval '1 second'),
    last_error = sqlc.arg(last_error)
WHERE id = sqlc.arg(id)
  AND processed_at IS NULL
  AND dead_lettered_at IS NULL;

-- name: DeadLetterEmailProviderEvent :execrows
UPDATE email_provider_events
SET dead_lettered_at = COALESCE(dead_lettered_at, now()),
    next_attempt_at = NULL,
    last_error = sqlc.arg(last_error)
WHERE id = sqlc.arg(id)
  AND processed_at IS NULL;
