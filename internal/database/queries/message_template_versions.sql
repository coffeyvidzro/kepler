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
