-- name: CreateContactProperty :one
INSERT INTO contact_properties (
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number
) VALUES (
    sqlc.arg(team_id),
    sqlc.arg(key),
    sqlc.arg(value_type),
    sqlc.narg(fallback_string),
    sqlc.narg(fallback_number)
)
RETURNING *;

-- name: ListContactProperties :many
SELECT *
FROM contact_properties
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListContactPropertiesAfter :many
SELECT property.*
FROM contact_properties AS property
WHERE property.team_id = sqlc.arg(scope_team_id)
  AND (property.created_at, property.id) < (
      SELECT cursor_property.created_at, cursor_property.id
      FROM contact_properties AS cursor_property
      WHERE cursor_property.id = sqlc.arg(cursor_id)
        AND cursor_property.team_id = sqlc.arg(scope_team_id)
  )
ORDER BY property.created_at DESC, property.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListContactPropertiesBefore :many
SELECT property.*
FROM contact_properties AS property
WHERE property.team_id = sqlc.arg(scope_team_id)
  AND (property.created_at, property.id) > (
      SELECT cursor_property.created_at, cursor_property.id
      FROM contact_properties AS cursor_property
      WHERE cursor_property.id = sqlc.arg(cursor_id)
        AND cursor_property.team_id = sqlc.arg(scope_team_id)
  )
ORDER BY property.created_at ASC, property.id ASC
LIMIT sqlc.arg(page_limit);

-- name: ContactPropertyCursorExists :one
SELECT EXISTS (
    SELECT 1
    FROM contact_properties
    WHERE id = sqlc.arg(cursor_id)
      AND team_id = sqlc.arg(team_id)
);

-- name: GetContactProperty :one
SELECT *
FROM contact_properties
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetContactPropertyByKey :one
SELECT *
FROM contact_properties
WHERE key = sqlc.arg(key)
  AND team_id = sqlc.arg(team_id);

-- name: UpdateContactPropertyFallback :one
UPDATE contact_properties
SET fallback_string = sqlc.narg(fallback_string),
    fallback_number = sqlc.narg(fallback_number),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: DeleteContactProperty :one
DELETE FROM contact_properties
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;
