-- name: CreateVerificationAttempt :one
INSERT INTO verification_attempts (
    team_id, verification_id, challenge_id, result,
    ip_address_hash, user_agent, metadata
)
SELECT
    challenge.team_id,
    challenge.verification_id,
    challenge.id,
    sqlc.arg(result),
    sqlc.narg(ip_address_hash),
    sqlc.narg(user_agent),
    sqlc.arg(metadata)
FROM verification_challenges AS challenge
WHERE challenge.id = sqlc.arg(challenge_id)
  AND challenge.verification_id = sqlc.arg(verification_id)
  AND challenge.team_id = sqlc.arg(team_id)
RETURNING *;

-- name: ListVerificationAttempts :many
SELECT *
FROM verification_attempts
WHERE verification_id = sqlc.arg(verification_id)
  AND team_id = sqlc.arg(team_id)
ORDER BY attempted_at DESC, id DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);
