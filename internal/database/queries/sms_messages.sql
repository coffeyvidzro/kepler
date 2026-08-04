-- name: CreateSMSMessage :one
INSERT INTO sms_messages (
    team_id,
    sender_id,
    to_number,
    from_name,
    body,
    status,
    segments,
    metadata,
    tags,
    scheduled_at,
    destination_country
) VALUES (
    sqlc.arg(team_id),
    sqlc.narg(sender_id),
    sqlc.arg(to_number),
    sqlc.arg(from_name),
    sqlc.arg(body),
    sqlc.arg(status),
    sqlc.arg(segments),
    sqlc.arg(metadata),
    sqlc.arg(tags),
    sqlc.narg(scheduled_at),
    sqlc.arg(destination_country)
)
RETURNING *;

-- name: ListSMSMessages :many
SELECT *
FROM sms_messages
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: GetSMSMessage :one
SELECT *
FROM sms_messages
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: MarkSMSMessageSubmitted :one
UPDATE sms_messages
SET provider_id = sqlc.arg(provider_id),
    provider_message_id = sqlc.arg(provider_message_id),
    status = sqlc.arg(status),
    error_message = NULL,
    submitted_at = COALESCE(submitted_at, now()),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: MarkSMSMessageFailed :one
UPDATE sms_messages
SET status = 'failed',
    error_message = sqlc.arg(error_message),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: UpdateSMSMessageStatus :one
UPDATE sms_messages
SET status = sqlc.arg(status),
    delivered_at = CASE
        WHEN sqlc.arg(status) = 'delivered'
            THEN COALESCE(delivered_at, now())
        ELSE delivered_at
    END,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status <> sqlc.arg(status)
  AND status NOT IN ('delivered', 'undelivered', 'rejected', 'failed', 'expired', 'unknown', 'canceled')
  AND (
      sqlc.arg(status) IN ('delivered', 'undelivered', 'rejected', 'failed', 'expired')
      OR CASE status
          WHEN 'queued' THEN 0
          WHEN 'processing' THEN 1
          WHEN 'submitted' THEN 2
          WHEN 'sent' THEN 3
          ELSE 4
      END < CASE sqlc.arg(status)
          WHEN 'queued' THEN 0
          WHEN 'processing' THEN 1
          WHEN 'submitted' THEN 2
          WHEN 'sent' THEN 3
          ELSE -1
      END
  )
RETURNING *;

-- name: FindApprovedSMSSender :one
SELECT id
FROM sender_ids
WHERE team_id = sqlc.arg(team_id)
  AND lower(name) = lower(sqlc.arg(name))
  AND status = 'approved'
ORDER BY created_at DESC
LIMIT 1;

-- name: MarkSMSMessageProcessing :one
UPDATE sms_messages
SET status = 'processing',
    error_message = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'queued'
  AND provider_message_id IS NULL
RETURNING *;

-- name: MarkSMSMessageDeliveryUnknown :one
UPDATE sms_messages
SET status = 'unknown',
    error_message = sqlc.arg(error_message),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'processing'
  AND provider_message_id IS NULL
RETURNING *;
