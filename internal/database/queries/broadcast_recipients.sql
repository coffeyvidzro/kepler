-- name: CreateBroadcastRecipient :one
INSERT INTO broadcast_recipients (
    team_id, broadcast_id, contact_id, email, normalized_email,
    first_name, last_name, contact_snapshot, status, exclusion_reason
) VALUES (
    sqlc.arg(team_id), sqlc.arg(broadcast_id), sqlc.narg(contact_id),
    sqlc.arg(email), sqlc.arg(normalized_email), sqlc.narg(first_name),
    sqlc.narg(last_name), sqlc.arg(contact_snapshot), sqlc.arg(status),
    sqlc.narg(exclusion_reason)
)
RETURNING *;

-- name: ListBroadcastRecipients :many
SELECT *
FROM broadcast_recipients
WHERE team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id)
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: SetBroadcastRecipientQueued :one
UPDATE broadcast_recipients
SET status = 'queued', email_message_id = sqlc.arg(email_message_id), queued_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND broadcast_id = sqlc.arg(broadcast_id)
  AND status = 'pending'
RETURNING *;
