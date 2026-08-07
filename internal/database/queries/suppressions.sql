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
RETURNING id,
          team_id,
          email,
          origin,
          source_id,
          created_at;

-- name: CreateSuppressions :many
INSERT INTO suppressions (
    team_id,
    email,
    origin
)
SELECT sqlc.arg(team_id),
       lower(batch_email.email),
       'manual'
FROM unnest(sqlc.arg(emails)::text[]) WITH ORDINALITY AS batch_email(email, position)
ORDER BY batch_email.position
RETURNING id,
          team_id,
          email,
          origin,
          source_id,
          created_at;

-- name: ListSuppressions :many
SELECT s.id,
       s.team_id,
       s.email,
       s.origin,
       s.source_id,
       s.created_at
FROM suppressions AS s
WHERE s.team_id = sqlc.arg(team_id)
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: ListSuppressionsFiltered :many
SELECT s.id,
       s.team_id,
       s.email,
       s.origin,
       s.source_id,
       s.created_at
FROM suppressions AS s
WHERE s.team_id = sqlc.arg(team_id)
  AND (sqlc.narg(filter_origin)::text IS NULL OR s.origin = sqlc.narg(filter_origin))
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListSuppressionsAfter :many
SELECT s.id,
       s.team_id,
       s.email,
       s.origin,
       s.source_id,
       s.created_at
FROM suppressions AS s
WHERE s.team_id = sqlc.arg(scope_team_id)
  AND (sqlc.narg(filter_origin)::text IS NULL OR s.origin = sqlc.narg(filter_origin))
  AND (s.created_at, s.id) < (
      SELECT cursor_suppression.created_at, cursor_suppression.id
      FROM suppressions AS cursor_suppression
      WHERE cursor_suppression.id = sqlc.arg(cursor_id)
        AND cursor_suppression.team_id = sqlc.arg(scope_team_id)
  )
ORDER BY s.created_at DESC, s.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListSuppressionsBefore :many
SELECT s.id,
       s.team_id,
       s.email,
       s.origin,
       s.source_id,
       s.created_at
FROM suppressions AS s
WHERE s.team_id = sqlc.arg(scope_team_id)
  AND (sqlc.narg(filter_origin)::text IS NULL OR s.origin = sqlc.narg(filter_origin))
  AND (s.created_at, s.id) > (
      SELECT cursor_suppression.created_at, cursor_suppression.id
      FROM suppressions AS cursor_suppression
      WHERE cursor_suppression.id = sqlc.arg(cursor_id)
        AND cursor_suppression.team_id = sqlc.arg(scope_team_id)
  )
ORDER BY s.created_at ASC, s.id ASC
LIMIT sqlc.arg(page_limit);

-- name: SuppressionCursorExists :one
SELECT EXISTS (
    SELECT 1
    FROM suppressions AS s
    WHERE s.id = sqlc.arg(cursor_id)
      AND s.team_id = sqlc.arg(team_id)
);

-- name: GetSuppressionByID :one
SELECT s.id,
       s.team_id,
       s.email,
       s.origin,
       s.source_id,
       s.created_at
FROM suppressions AS s
WHERE s.id = sqlc.arg(id)
  AND s.team_id = sqlc.arg(team_id);

-- name: GetSuppressionByEmail :one
SELECT s.id,
       s.team_id,
       s.email,
       s.origin,
       s.source_id,
       s.created_at
FROM suppressions AS s
WHERE s.team_id = sqlc.arg(team_id)
  AND lower(s.email) = lower(sqlc.arg(email));

-- name: DeleteSuppressionByID :one
DELETE FROM suppressions
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING id,
          team_id,
          email,
          origin,
          source_id,
          created_at;

-- name: DeleteSuppressionByEmail :one
DELETE FROM suppressions
WHERE team_id = sqlc.arg(team_id)
  AND lower(email) = lower(sqlc.arg(email))
RETURNING id,
          team_id,
          email,
          origin,
          source_id,
          created_at;

-- name: DeleteSuppressionsByIDs :many
DELETE FROM suppressions
WHERE team_id = sqlc.arg(team_id)
  AND id = ANY(sqlc.arg(ids)::uuid[])
RETURNING id,
          team_id,
          email,
          origin,
          source_id,
          created_at;

-- name: DeleteSuppressionsByEmails :many
DELETE FROM suppressions
WHERE team_id = sqlc.arg(team_id)
  AND lower(email) = ANY(sqlc.arg(emails)::text[])
RETURNING id,
          team_id,
          email,
          origin,
          source_id,
          created_at;
