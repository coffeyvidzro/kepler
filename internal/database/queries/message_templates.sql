-- name: CreateMessageTemplate :one
INSERT INTO message_templates (team_id, name, alias)
VALUES (sqlc.arg(team_id), sqlc.arg(name), sqlc.narg(alias))
RETURNING id,
          team_id,
          name,
          alias,
          current_version_id,
          published_version_id,
          next_version_number,
          published_at,
          created_at,
          updated_at,
          deleted_at;

-- name: ListMessageTemplates :many
SELECT mt.id,
       mt.team_id,
       mt.name,
       mt.alias,
       mt.current_version_id,
       mt.published_version_id,
       mt.next_version_number,
       mt.published_at,
       mt.created_at,
       mt.updated_at,
       mt.deleted_at
FROM message_templates AS mt
WHERE mt.team_id = sqlc.arg(team_id)
  AND mt.deleted_at IS NULL
ORDER BY mt.created_at DESC, mt.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: ListMessageTemplatesAfter :many
SELECT mt.id,
       mt.team_id,
       mt.name,
       mt.alias,
       mt.current_version_id,
       mt.published_version_id,
       mt.next_version_number,
       mt.published_at,
       mt.created_at,
       mt.updated_at,
       mt.deleted_at
FROM message_templates AS mt
WHERE mt.team_id = sqlc.arg(scope_team_id)
  AND mt.deleted_at IS NULL
  AND (mt.created_at, mt.id) < (
      SELECT cursor_template.created_at, cursor_template.id
      FROM message_templates AS cursor_template
      WHERE cursor_template.id = sqlc.arg(cursor_id)
        AND cursor_template.team_id = sqlc.arg(scope_team_id)
        AND cursor_template.deleted_at IS NULL
  )
ORDER BY mt.created_at DESC, mt.id DESC
LIMIT sqlc.arg(page_limit);

-- name: ListMessageTemplatesBefore :many
SELECT mt.id,
       mt.team_id,
       mt.name,
       mt.alias,
       mt.current_version_id,
       mt.published_version_id,
       mt.next_version_number,
       mt.published_at,
       mt.created_at,
       mt.updated_at,
       mt.deleted_at
FROM message_templates AS mt
WHERE mt.team_id = sqlc.arg(scope_team_id)
  AND mt.deleted_at IS NULL
  AND (mt.created_at, mt.id) > (
      SELECT cursor_template.created_at, cursor_template.id
      FROM message_templates AS cursor_template
      WHERE cursor_template.id = sqlc.arg(cursor_id)
        AND cursor_template.team_id = sqlc.arg(scope_team_id)
        AND cursor_template.deleted_at IS NULL
  )
ORDER BY mt.created_at ASC, mt.id ASC
LIMIT sqlc.arg(page_limit);

-- name: MessageTemplateCursorExists :one
SELECT EXISTS (
    SELECT 1
    FROM message_templates AS mt
    WHERE mt.id = sqlc.arg(cursor_id)
      AND mt.team_id = sqlc.arg(team_id)
      AND mt.deleted_at IS NULL
);

-- name: GetMessageTemplateByID :one
SELECT mt.id,
       mt.team_id,
       mt.name,
       mt.alias,
       mt.current_version_id,
       mt.published_version_id,
       mt.next_version_number,
       mt.published_at,
       mt.created_at,
       mt.updated_at,
       mt.deleted_at
FROM message_templates AS mt
WHERE mt.id = sqlc.arg(id)
  AND mt.team_id = sqlc.arg(team_id)
  AND mt.deleted_at IS NULL;

-- name: GetMessageTemplateByAlias :one
SELECT mt.id,
       mt.team_id,
       mt.name,
       mt.alias,
       mt.current_version_id,
       mt.published_version_id,
       mt.next_version_number,
       mt.published_at,
       mt.created_at,
       mt.updated_at,
       mt.deleted_at
FROM message_templates AS mt
WHERE mt.team_id = sqlc.arg(team_id)
  AND lower(mt.alias) = lower(sqlc.arg(alias))
  AND mt.deleted_at IS NULL;

-- name: LockMessageTemplate :one
SELECT mt.id,
       mt.team_id,
       mt.name,
       mt.alias,
       mt.current_version_id,
       mt.published_version_id,
       mt.next_version_number,
       mt.published_at,
       mt.created_at,
       mt.updated_at,
       mt.deleted_at
FROM message_templates AS mt
WHERE mt.id = sqlc.arg(id)
  AND mt.team_id = sqlc.arg(team_id)
  AND mt.deleted_at IS NULL
FOR UPDATE;

-- name: UpdateMessageTemplateMetadata :one
UPDATE message_templates
SET name = sqlc.arg(name),
    alias = sqlc.narg(alias),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING id,
          team_id,
          name,
          alias,
          current_version_id,
          published_version_id,
          next_version_number,
          published_at,
          created_at,
          updated_at,
          deleted_at;

-- name: SetMessageTemplateCurrentVersion :one
UPDATE message_templates
SET current_version_id = sqlc.arg(version_id),
    next_version_number = next_version_number + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING id,
          team_id,
          name,
          alias,
          current_version_id,
          published_version_id,
          next_version_number,
          published_at,
          created_at,
          updated_at,
          deleted_at;

-- name: PublishMessageTemplateVersion :one
UPDATE message_templates
SET published_version_id = sqlc.arg(version_id),
    published_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING id,
          team_id,
          name,
          alias,
          current_version_id,
          published_version_id,
          next_version_number,
          published_at,
          created_at,
          updated_at,
          deleted_at;

-- name: SoftDeleteMessageTemplate :one
UPDATE message_templates
SET deleted_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND deleted_at IS NULL
RETURNING id,
          team_id,
          name,
          alias,
          current_version_id,
          published_version_id,
          next_version_number,
          published_at,
          created_at,
          updated_at,
          deleted_at;

-- name: CreateMessageTemplatePublication :one
INSERT INTO message_template_publications (team_id, template_id, version_id)
VALUES (sqlc.arg(team_id), sqlc.arg(template_id), sqlc.arg(version_id))
RETURNING id, team_id, template_id, version_id, published_at;
