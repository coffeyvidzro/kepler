-- name: CreateTeamToken :one
INSERT INTO team_tokens (
    team_id,
    name,
    token_hash,
    token_prefix,
    permissions,
    created_by,
    expires_at
)
SELECT
    team.id,
    sqlc.arg(name),
    sqlc.arg(token_hash),
    sqlc.arg(token_prefix),
    sqlc.arg(permissions),
    sqlc.narg(created_by),
    sqlc.narg(expires_at)
FROM teams AS team
WHERE team.id = sqlc.arg(team_id)
  AND team.status = 'active'
RETURNING *;

-- name: GetActiveTeamTokenByHash :one
SELECT tt.*
FROM team_tokens tt
JOIN teams t ON t.id = tt.team_id
WHERE tt.token_hash = sqlc.arg(token_hash)
  AND tt.revoked_at IS NULL
  AND (tt.expires_at IS NULL OR tt.expires_at > now())
  AND t.status = 'active';

-- name: ListTeamTokens :many
SELECT token.*
FROM team_tokens AS token
JOIN teams AS team ON team.id = token.team_id
WHERE token.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY token.created_at DESC;

-- name: UpdateTeamToken :one
UPDATE team_tokens AS token
SET name = sqlc.arg(name),
    permissions = sqlc.arg(permissions),
    expires_at = sqlc.narg(expires_at),
    updated_at = now()
FROM teams AS team
WHERE token.id = sqlc.arg(id)
  AND token.team_id = sqlc.arg(team_id)
  AND team.id = token.team_id
  AND team.status = 'active'
  AND token.revoked_at IS NULL
RETURNING token.*;

-- name: RevokeTeamToken :one
UPDATE team_tokens AS token
SET revoked_at = now(),
    updated_at = now()
FROM teams AS team
WHERE token.id = sqlc.arg(id)
  AND token.team_id = sqlc.arg(team_id)
  AND team.id = token.team_id
  AND team.status = 'active'
  AND token.revoked_at IS NULL
RETURNING token.*;

-- name: TouchTeamToken :exec
UPDATE team_tokens
SET last_used_at = now()
WHERE id = sqlc.arg(id)
  AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes');
