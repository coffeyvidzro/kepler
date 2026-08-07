-- name: CreateSuppression :one
INSERT INTO suppressions (
    team_id,
    email,
    origin,
    source_id
) VALUES (
    sqlc.arg(team_id),
    sqlc.arg(email),
    sqlc.arg(origin),
    sqlc.narg(source_id)
)
RETURNING *;

-- name: ListSuppressions :many
SELECT *
FROM suppressions
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetSuppressionByID :one
SELECT *
FROM suppressions
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetSuppressionByEmail :one
SELECT *
FROM suppressions
WHERE team_id = sqlc.arg(team_id)
  AND lower(email) = lower(sqlc.arg(email));

-- name: DeleteSuppressionByID :one
DELETE FROM suppressions
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: DeleteSuppressionByEmail :one
DELETE FROM suppressions
WHERE team_id = sqlc.arg(team_id)
  AND lower(email) = lower(sqlc.arg(email))
RETURNING *;
