-- name: CreateMessageTemplate :one
INSERT INTO message_templates (team_id, name, alias)
VALUES (sqlc.arg(team_id), sqlc.arg(name), sqlc.arg(alias))
RETURNING *;

-- name: ListMessageTemplates :many
SELECT *
FROM message_templates
WHERE team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetMessageTemplateByID :one
SELECT *
FROM message_templates
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL;

-- name: GetMessageTemplateByAlias :one
SELECT *
FROM message_templates
WHERE team_id = sqlc.arg(team_id)
  AND lower(alias) = lower(sqlc.arg(alias))
  AND deleted_at IS NULL;

-- name: LockMessageTemplate :one
SELECT *
FROM message_templates
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
FOR UPDATE;

-- name: UpdateMessageTemplateMetadata :one
UPDATE message_templates
SET name = sqlc.arg(name),
    alias = sqlc.arg(alias),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING *;

-- name: SetMessageTemplateCurrentVersion :one
UPDATE message_templates
SET current_version_id = sqlc.arg(version_id),
    next_version_number = next_version_number + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING *;

-- name: PublishMessageTemplateVersion :one
UPDATE message_templates
SET published_version_id = sqlc.arg(version_id),
    published_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteMessageTemplate :one
UPDATE message_templates
SET deleted_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING *;

-- name: CreateMessageTemplatePublication :one
INSERT INTO message_template_publications (team_id, template_id, version_id)
VALUES (sqlc.arg(team_id), sqlc.arg(template_id), sqlc.arg(version_id))
RETURNING *;
