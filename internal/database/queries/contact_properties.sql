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
RETURNING
    id,
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number::text AS fallback_number,
    created_at,
    updated_at;

-- name: ListContactProperties :many
SELECT
    id,
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number::text AS fallback_number,
    created_at,
    updated_at
FROM contact_properties
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListContactPropertiesAfter :many
WITH cursor AS (
    SELECT created_at, id
    FROM contact_properties
    WHERE id = sqlc.arg(cursor_id)
      AND team_id = sqlc.arg(team_id)
)
SELECT
    property.id,
    property.team_id,
    property.key,
    property.value_type,
    property.fallback_string,
    property.fallback_number::text AS fallback_number,
    property.created_at,
    property.updated_at
FROM contact_properties AS property
CROSS JOIN cursor
WHERE property.team_id = sqlc.arg(team_id)
  AND (property.created_at, property.id) < (cursor.created_at, cursor.id)
ORDER BY property.created_at DESC, property.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListContactPropertiesBefore :many
WITH cursor AS (
    SELECT created_at, id
    FROM contact_properties
    WHERE id = sqlc.arg(cursor_id)
      AND team_id = sqlc.arg(team_id)
), page AS (
    SELECT
        property.id,
        property.team_id,
        property.key,
        property.value_type,
        property.fallback_string,
        property.fallback_number::text AS fallback_number,
        property.created_at,
        property.updated_at
    FROM contact_properties AS property
    CROSS JOIN cursor
    WHERE property.team_id = sqlc.arg(team_id)
      AND (property.created_at, property.id) > (cursor.created_at, cursor.id)
    ORDER BY property.created_at ASC, property.id ASC
    LIMIT sqlc.arg(page_limit)
)
SELECT
    id,
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number,
    created_at,
    updated_at
FROM page
ORDER BY created_at DESC, id DESC;

-- name: ContactPropertyCursorExists :one
SELECT EXISTS (
    SELECT 1
    FROM contact_properties
    WHERE id = sqlc.arg(cursor_id)
      AND team_id = sqlc.arg(team_id)
);

-- name: GetContactProperty :one
SELECT
    id,
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number::text AS fallback_number,
    created_at,
    updated_at
FROM contact_properties
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetContactPropertyByKey :one
SELECT
    id,
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number::text AS fallback_number,
    created_at,
    updated_at
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
RETURNING
    id,
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number::text AS fallback_number,
    created_at,
    updated_at;

-- name: DeleteContactProperty :one
DELETE FROM contact_properties
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING
    id,
    team_id,
    key,
    value_type,
    fallback_string,
    fallback_number::text AS fallback_number,
    created_at,
    updated_at;
