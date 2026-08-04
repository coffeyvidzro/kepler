-- name: CreateEmailRecipient :one
INSERT INTO email_recipients (
    email_message_id,
    recipient_email,
    recipient_type,
    status
) VALUES (
    sqlc.arg(email_message_id),
    lower(trim(sqlc.arg(recipient_email))),
    sqlc.arg(recipient_type),
    sqlc.arg(status)
)
ON CONFLICT (email_message_id, recipient_email) DO UPDATE
SET recipient_type = EXCLUDED.recipient_type,
    updated_at = now()
RETURNING *;

-- name: GetEmailRecipientForUpdate :one
SELECT *
FROM email_recipients
WHERE email_message_id = sqlc.arg(email_message_id)
  AND recipient_email = lower(trim(sqlc.arg(recipient_email)))
FOR UPDATE;

-- name: UpdateEmailRecipientState :execrows
UPDATE email_recipients
SET status = sqlc.arg(status),
    last_event_type = sqlc.narg(last_event_type),
    last_event_at = sqlc.narg(last_event_at),
    last_action = sqlc.narg(last_action),
    last_status_code = sqlc.narg(last_status_code),
    last_diagnostic_code = sqlc.narg(last_diagnostic_code),
    delivered_at = COALESCE(sqlc.narg(delivered_at), delivered_at),
    failed_at = COALESCE(sqlc.narg(failed_at), failed_at),
    error_code = sqlc.narg(error_code),
    error_message = sqlc.narg(error_message),
    updated_at = now()
WHERE email_message_id = sqlc.arg(email_message_id)
  AND recipient_email = lower(trim(sqlc.arg(recipient_email)));

-- name: ListEmailRecipientsByMessage :many
SELECT *
FROM email_recipients
WHERE email_message_id = sqlc.arg(email_message_id)
ORDER BY recipient_type, recipient_email;

-- name: ListEmailRecipientLifecycleDetails :many
SELECT
    recipient_email,
    status,
    COALESCE(last_action, '') AS last_action,
    COALESCE(last_status_code, '') AS last_status_code,
    COALESCE(last_diagnostic_code, '') AS last_diagnostic_code
FROM email_recipients
WHERE email_message_id = sqlc.arg(email_message_id)
  AND recipient_email = ANY(sqlc.arg(recipient_emails)::text[])
ORDER BY recipient_email;

-- name: ListEmailRecipientAggregateStates :many
SELECT status, delivered_at, failed_at
FROM email_recipients
WHERE email_message_id = sqlc.arg(email_message_id)
FOR SHARE;
