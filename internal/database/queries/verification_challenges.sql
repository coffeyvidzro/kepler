-- name: CreateVerificationChallenge :one
INSERT INTO verification_challenges (
    team_id, verification_id, sequence, code_hmac, status, channel, expires_at
)
SELECT
    verification.team_id, verification.id, sqlc.arg(sequence), sqlc.arg(code_hmac),
    'queued', sqlc.arg(channel), sqlc.arg(expires_at)
FROM verifications AS verification
WHERE verification.id = sqlc.arg(verification_id)
  AND verification.team_id = sqlc.arg(team_id)
  AND verification.status = 'pending'
RETURNING *;

-- name: GetVerificationChallenge :one
SELECT *
FROM verification_challenges
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetActiveVerificationChallengeForUpdate :one
SELECT *
FROM verification_challenges
WHERE verification_id = sqlc.arg(verification_id)
  AND team_id = sqlc.arg(team_id)
  AND status IN ('queued', 'dispatching', 'dispatched')
ORDER BY sequence DESC
LIMIT 1
FOR UPDATE;

-- name: ListVerificationChallenges :many
SELECT *
FROM verification_challenges
WHERE verification_id = sqlc.arg(verification_id)
  AND team_id = sqlc.arg(team_id)
ORDER BY sequence DESC;

-- name: SupersedeActiveVerificationChallenges :many
UPDATE verification_challenges
SET status = 'superseded',
    superseded_at = now(),
    updated_at = now()
WHERE verification_id = sqlc.arg(verification_id)
  AND team_id = sqlc.arg(team_id)
  AND status IN ('queued', 'dispatching', 'dispatched')
RETURNING *;

-- name: MarkVerificationChallengeDispatching :one
UPDATE verification_challenges
SET status = 'dispatching',
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'queued'
RETURNING *;

-- name: MarkVerificationChallengeEmailDispatched :one
UPDATE verification_challenges
SET status = 'dispatched',
    email_message_id = sqlc.arg(email_message_id),
    dispatched_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND channel = 'email'
  AND status IN ('queued', 'dispatching')
RETURNING *;

-- name: MarkVerificationChallengeSMSDispatched :one
UPDATE verification_challenges
SET status = 'dispatched',
    sms_message_id = sqlc.arg(sms_message_id),
    dispatched_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND channel = 'sms'
  AND status IN ('queued', 'dispatching')
RETURNING *;

-- name: MarkVerificationChallengeDeliveryFailed :one
UPDATE verification_challenges
SET status = 'delivery_failed',
    delivery_failed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status IN ('queued', 'dispatching')
RETURNING *;

-- name: MarkVerificationChallengeExpired :one
UPDATE verification_challenges
SET status = 'expired',
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status IN ('queued', 'dispatching', 'dispatched')
RETURNING *;
