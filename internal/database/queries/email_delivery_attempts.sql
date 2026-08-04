-- name: CreateEmailDeliveryAttempt :one
INSERT INTO email_delivery_attempts (
    email_message_id,
    team_id,
    attempt_number,
    status,
    provider
) VALUES (
    sqlc.arg(email_message_id),
    sqlc.arg(team_id),
    sqlc.arg(attempt_number),
    sqlc.arg(status),
    sqlc.arg(provider)
)
RETURNING *;

-- name: GetNextEmailDeliveryAttemptNumber :one
SELECT COALESCE(MAX(attempt_number), 0)::integer + 1
FROM email_delivery_attempts
WHERE email_message_id = sqlc.arg(email_message_id);

-- name: GetEmailDeliveryAttempt :one
SELECT *
FROM email_delivery_attempts
WHERE id = sqlc.arg(id)
  AND email_message_id = sqlc.arg(email_message_id)
  AND team_id = sqlc.arg(team_id);

-- name: EmailDeliveryAttemptExists :one
SELECT EXISTS (
    SELECT 1
    FROM email_delivery_attempts
    WHERE id = sqlc.arg(id)
      AND email_message_id = sqlc.arg(email_message_id)
);

-- name: MarkEmailDeliveryAttemptRequestStarted :execrows
UPDATE email_delivery_attempts
SET status = 'request_started',
    request_started_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND email_message_id = sqlc.arg(email_message_id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'claimed';

-- name: MarkEmailDeliveryAttemptSubmitted :execrows
UPDATE email_delivery_attempts
SET status = 'submitted',
    provider = sqlc.arg(provider),
    provider_message_id = sqlc.arg(provider_message_id),
    completed_at = now(),
    error_code = NULL,
    error_message = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND email_message_id = sqlc.arg(email_message_id)
  AND team_id = sqlc.arg(team_id)
  AND status IN ('claimed', 'request_started', 'submission_unknown');

-- name: CompleteEmailDeliveryAttempt :execrows
UPDATE email_delivery_attempts
SET status = sqlc.arg(status),
    error_code = sqlc.narg(error_code),
    error_message = sqlc.narg(error_message),
    completed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND email_message_id = sqlc.arg(email_message_id)
  AND team_id = sqlc.arg(team_id)
  AND status IN ('claimed', 'request_started');

-- name: ListEmailDeliveryAttempts :many
SELECT *
FROM email_delivery_attempts
WHERE email_message_id = sqlc.arg(email_message_id)
ORDER BY attempt_number DESC;
