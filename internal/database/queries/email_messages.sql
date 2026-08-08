-- name: CreateEmailMessage :one
INSERT INTO email_messages (
    team_id,
    sender_domain_id,
    delivery_provider,
    provider_region,
    message_type,
    from_email,
    from_name,
    reply_to_email,
    to_email,
    to_name,
    subject,
    html_body,
    text_body,
    status,
    metadata,
    recipients,
    headers,
    attachments,
    tags,
    scheduled_at
)
SELECT
    team.id,
    sqlc.narg(sender_domain_id),
    sqlc.arg(delivery_provider),
    sqlc.arg(provider_region),
    sqlc.arg(message_type),
    sqlc.arg(from_email),
    sqlc.narg(from_name),
    sqlc.narg(reply_to_email),
    sqlc.arg(to_email),
    sqlc.narg(to_name),
    sqlc.arg(subject),
    sqlc.narg(html_body),
    sqlc.narg(text_body),
    'queued',
    sqlc.arg(metadata),
    sqlc.arg(recipients),
    sqlc.arg(headers),
    sqlc.arg(attachments),
    sqlc.arg(tags),
    sqlc.narg(scheduled_at)
FROM teams AS team
WHERE team.id = sqlc.arg(team_id)
  AND team.status = 'active'
RETURNING *;

-- name: GetEmailMessage :one
SELECT message.*
FROM email_messages AS message
JOIN teams AS team ON team.id = message.team_id
WHERE message.id = sqlc.arg(id)
  AND message.team_id = sqlc.arg(team_id)
  AND team.status = 'active';

-- name: ListEmailMessages :many
SELECT
    message.id,
    message.team_id,
    message.to_email,
    message.to_name,
    message.subject,
    message.status,
    message.provider,
    message.queued_at,
    message.submitted_at,
    message.delivered_at,
    message.created_at
FROM email_messages AS message
JOIN teams AS team ON team.id = message.team_id
WHERE message.team_id = sqlc.arg(team_id)
  AND team.status = 'active'
ORDER BY message.created_at DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);
