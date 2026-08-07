-- name: CreateMessageTemplateVersion :one
INSERT INTO message_template_versions (
    team_id,
    template_id,
    version_number,
    from_email,
    from_name,
    reply_to_email,
    subject,
    html_body,
    text_body,
    variables,
    based_on_version_id,
    change_note
) VALUES (
    sqlc.arg(team_id),
    sqlc.arg(template_id),
    sqlc.arg(version_number),
    sqlc.narg(from_email),
    sqlc.narg(from_name),
    sqlc.narg(reply_to_email),
    sqlc.arg(subject),
    sqlc.arg(html_body),
    sqlc.narg(text_body),
    sqlc.arg(variables),
    sqlc.narg(based_on_version_id),
    sqlc.narg(change_note)
)
RETURNING id,
          team_id,
          template_id,
          version_number,
          from_email,
          from_name,
          reply_to_email,
          subject,
          html_body,
          text_body,
          variables,
          based_on_version_id,
          change_note,
          created_at;

-- name: ListMessageTemplateVersions :many
SELECT mtv.id,
       mtv.team_id,
       mtv.template_id,
       mtv.version_number,
       mtv.from_email,
       mtv.from_name,
       mtv.reply_to_email,
       mtv.subject,
       mtv.html_body,
       mtv.text_body,
       mtv.variables,
       mtv.based_on_version_id,
       mtv.change_note,
       mtv.created_at
FROM message_template_versions AS mtv
WHERE mtv.team_id = sqlc.arg(team_id)
  AND mtv.template_id = sqlc.arg(template_id)
ORDER BY mtv.version_number DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetMessageTemplateVersion :one
SELECT mtv.id,
       mtv.team_id,
       mtv.template_id,
       mtv.version_number,
       mtv.from_email,
       mtv.from_name,
       mtv.reply_to_email,
       mtv.subject,
       mtv.html_body,
       mtv.text_body,
       mtv.variables,
       mtv.based_on_version_id,
       mtv.change_note,
       mtv.created_at
FROM message_template_versions AS mtv
WHERE mtv.id = sqlc.arg(id)
  AND mtv.template_id = sqlc.arg(template_id)
  AND mtv.team_id = sqlc.arg(team_id);

-- name: MessageTemplateVersionExists :one
SELECT EXISTS (
    SELECT 1
    FROM message_template_versions AS mtv
    WHERE mtv.id = sqlc.arg(version_id)
      AND mtv.template_id = sqlc.arg(template_id)
      AND mtv.team_id = sqlc.arg(team_id)
);
