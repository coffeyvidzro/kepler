-- name: CreateTopic :one
INSERT INTO topics (
    team_id,
    name,
    description,
    default_subscription,
    visibility
) VALUES (
    sqlc.arg(team_id),
    sqlc.arg(name),
    sqlc.narg(description),
    sqlc.arg(default_subscription),
    sqlc.arg(visibility)
)
RETURNING *;

-- name: ListTopics :many
SELECT *
FROM topics
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetTopic :one
SELECT *
FROM topics
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: UpdateTopic :one
UPDATE topics
SET name = sqlc.arg(name),
    description = sqlc.narg(description),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: DeleteTopic :one
DELETE FROM topics
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;
