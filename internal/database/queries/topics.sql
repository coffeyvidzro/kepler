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
RETURNING id,
          team_id,
          name,
          description,
          default_subscription,
          visibility,
          created_at,
          updated_at;

-- name: ListTopics :many
SELECT id,
       team_id,
       name,
       description,
       default_subscription,
       visibility,
       created_at,
       updated_at
FROM topics
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: ListTopicsAfter :many
SELECT id,
       team_id,
       name,
       description,
       default_subscription,
       visibility,
       created_at,
       updated_at
FROM topics
WHERE team_id = sqlc.arg(scope_team_id)
  AND (created_at, id) < (
      SELECT created_at, id
      FROM topics
      WHERE id = sqlc.arg(cursor_id)
        AND team_id = sqlc.arg(scope_team_id)
  )
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListTopicsBefore :many
SELECT id,
       team_id,
       name,
       description,
       default_subscription,
       visibility,
       created_at,
       updated_at
FROM topics
WHERE team_id = sqlc.arg(scope_team_id)
  AND (created_at, id) > (
      SELECT created_at, id
      FROM topics
      WHERE id = sqlc.arg(cursor_id)
        AND team_id = sqlc.arg(scope_team_id)
  )
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg(page_limit);

-- name: TopicCursorExists :one
SELECT EXISTS (
    SELECT 1
    FROM topics
    WHERE id = sqlc.arg(cursor_id)
      AND team_id = sqlc.arg(team_id)
);

-- name: GetTopic :one
SELECT id,
       team_id,
       name,
       description,
       default_subscription,
       visibility,
       created_at,
       updated_at
FROM topics
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: UpdateTopic :one
UPDATE topics
SET name = sqlc.arg(name),
    description = sqlc.narg(description),
    visibility = sqlc.arg(visibility),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING id,
          team_id,
          name,
          description,
          default_subscription,
          visibility,
          created_at,
          updated_at;

-- name: DeleteTopic :one
DELETE FROM topics
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING id,
          team_id,
          name,
          description,
          default_subscription,
          visibility,
          created_at,
          updated_at;
