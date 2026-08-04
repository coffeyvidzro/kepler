-- name: CreateVerificationService :one
INSERT INTO verification_services (
    team_id,
    key,
    name,
    default_channel,
    code_length,
    ttl_seconds,
    max_attempts,
    resend_cooldown_seconds,
    max_resends,
    enabled,
    metadata
)
SELECT
    team.id,
    sqlc.arg(key),
    sqlc.arg(name),
    sqlc.arg(default_channel),
    sqlc.arg(code_length),
    sqlc.arg(ttl_seconds),
    sqlc.arg(max_attempts),
    sqlc.arg(resend_cooldown_seconds),
    sqlc.arg(max_resends),
    sqlc.arg(enabled),
    sqlc.arg(metadata)
FROM teams AS team
WHERE team.id = sqlc.arg(team_id)
  AND team.status = 'active'
RETURNING *;

-- name: GetVerificationService :one
SELECT service.*
FROM verification_services AS service
JOIN teams AS team ON team.id = service.team_id
WHERE service.id = sqlc.arg(id)
  AND service.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: GetVerificationServiceByKey :one
SELECT service.*
FROM verification_services AS service
JOIN teams AS team ON team.id = service.team_id
WHERE service.team_id = sqlc.arg(team_id)
  AND service.key = sqlc.arg(key)
  AND team.status = 'active';

-- name: ListVerificationServices :many
SELECT service.*
FROM verification_services AS service
JOIN teams AS team ON team.id = service.team_id
WHERE service.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY service.created_at DESC, service.id DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: UpdateVerificationService :one
UPDATE verification_services AS service
SET name = sqlc.arg(name),
    default_channel = sqlc.arg(default_channel),
    code_length = sqlc.arg(code_length),
    ttl_seconds = sqlc.arg(ttl_seconds),
    max_attempts = sqlc.arg(max_attempts),
    resend_cooldown_seconds = sqlc.arg(resend_cooldown_seconds),
    max_resends = sqlc.arg(max_resends),
    metadata = sqlc.arg(metadata),
    updated_at = now()
FROM teams AS team
WHERE service.id = sqlc.arg(id)
  AND service.team_id = sqlc.arg(team_id)
  AND team.id = service.team_id
  AND team.status = 'active'
RETURNING service.*;

-- name: SetVerificationServiceEnabled :one
UPDATE verification_services AS service
SET enabled = sqlc.arg(enabled),
    updated_at = now()
FROM teams AS team
WHERE service.id = sqlc.arg(id)
  AND service.team_id = sqlc.arg(team_id)
  AND team.id = service.team_id
  AND team.status = 'active'
RETURNING service.*;
