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

-- name: CreateMessageTemplateVersion :one
INSERT INTO message_template_versions (
    team_id, template_id, version_number,
    from_email, from_name, reply_to_email,
    subject, html_body, text_body, variables,
    based_on_version_id, change_note
) VALUES (
    sqlc.arg(team_id), sqlc.arg(template_id), sqlc.arg(version_number),
    sqlc.narg(from_email), sqlc.narg(from_name), sqlc.narg(reply_to_email),
    sqlc.arg(subject), sqlc.arg(html_body), sqlc.narg(text_body), sqlc.arg(variables),
    sqlc.narg(based_on_version_id), sqlc.narg(change_note)
)
RETURNING *;

-- name: ListMessageTemplateVersions :many
SELECT *
FROM message_template_versions
WHERE team_id = sqlc.arg(team_id)
  AND template_id = sqlc.arg(template_id)
ORDER BY version_number DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetMessageTemplateVersion :one
SELECT *
FROM message_template_versions
WHERE id = sqlc.arg(id)
  AND template_id = sqlc.arg(template_id)
  AND team_id = sqlc.arg(team_id);

-- name: CreateMessageTemplatePublication :one
INSERT INTO message_template_publications (team_id, template_id, version_id)
VALUES (sqlc.arg(team_id), sqlc.arg(template_id), sqlc.arg(version_id))
RETURNING *;
