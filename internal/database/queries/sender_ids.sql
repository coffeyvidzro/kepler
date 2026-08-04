-- name: CreateSenderID :one
INSERT INTO sender_ids (
    team_id,
    name,
    country_code,
    purpose,
    provider,
    created_by
)
SELECT
    team.id,
    sqlc.arg(name),
    sqlc.arg(country_code),
    sqlc.arg(purpose),
    sqlc.narg(provider),
    sqlc.narg(created_by)
FROM teams AS team
WHERE team.id = sqlc.arg(team_id)
  AND team.status = 'active'
RETURNING *;

-- name: ListSenderIDs :many
SELECT sender.*
FROM sender_ids AS sender
JOIN teams AS team ON team.id = sender.team_id
WHERE sender.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY sender.created_at DESC;

-- name: GetSenderID :one
SELECT sender.*
FROM sender_ids AS sender
JOIN teams AS team ON team.id = sender.team_id
WHERE sender.id = sqlc.arg(id)
  AND sender.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: DeleteSenderID :one
DELETE FROM sender_ids AS sender
USING teams AS team
WHERE sender.id = sqlc.arg(id)
  AND sender.team_id = sqlc.arg(team_id)
  AND team.id = sender.team_id
  AND team.status = 'active'
RETURNING sender.*;
