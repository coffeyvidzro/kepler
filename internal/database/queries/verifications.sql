-- name: CreateVerification :one
INSERT INTO verifications (
    team_id, service_id, channel, recipient, recipient_normalized,
    status, locale, metadata, expires_at
)
SELECT
    service.team_id, service.id, sqlc.arg(channel), sqlc.arg(recipient),
    sqlc.arg(recipient_normalized), 'pending', sqlc.narg(locale),
    sqlc.arg(metadata), sqlc.arg(expires_at)
FROM verification_services AS service
JOIN teams AS team ON team.id = service.team_id
WHERE service.id = sqlc.arg(service_id)
  AND service.team_id = sqlc.arg(team_id)
  AND service.enabled = true
  AND team.status = 'active'
RETURNING *;

-- name: GetVerification :one
SELECT verification.*
FROM verifications AS verification
JOIN teams AS team ON team.id = verification.team_id
WHERE verification.id = sqlc.arg(id)
  AND verification.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: GetVerificationForUpdate :one
SELECT *
FROM verifications
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
FOR UPDATE;

-- name: ListVerifications :many
SELECT verification.*
FROM verifications AS verification
JOIN teams AS team ON team.id = verification.team_id
WHERE verification.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY verification.created_at DESC, verification.id DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: IncrementVerificationAttemptCount :one
UPDATE verifications
SET attempt_count = attempt_count + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'pending'
RETURNING *;

-- name: IncrementVerificationResendCount :one
UPDATE verifications
SET resend_count = resend_count + 1,
    expires_at = sqlc.arg(expires_at),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'pending'
RETURNING *;

-- name: ApproveVerification :one
UPDATE verifications
SET status = 'approved',
    approved_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'pending'
  AND expires_at > now()
RETURNING *;

-- name: ExpireVerification :one
UPDATE verifications
SET status = 'expired',
    expired_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'pending'
RETURNING *;

-- name: CancelVerification :one
UPDATE verifications
SET status = 'canceled',
    canceled_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'pending'
RETURNING *;

-- name: MarkVerificationMaxAttemptsReached :one
UPDATE verifications
SET status = 'max_attempts_reached',
    failed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'pending'
RETURNING *;

-- name: MarkVerificationDeliveryFailed :one
UPDATE verifications
SET status = 'delivery_failed',
    failed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'pending'
RETURNING *;

-- name: ExpirePendingVerifications :many
UPDATE verifications
SET status = 'expired',
    expired_at = now(),
    updated_at = now()
WHERE status = 'pending'
  AND expires_at <= now()
RETURNING *;
