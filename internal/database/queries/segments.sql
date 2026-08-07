-- name: CreateSegment :one
INSERT INTO segments (
    team_id,
    name
) VALUES (
    sqlc.arg(team_id),
    sqlc.arg(name)
)
RETURNING *;

-- name: ListSegments :many
SELECT *
FROM segments
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: GetSegment :one
SELECT *
FROM segments
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: ListSegmentContacts :many
SELECT
    contact.id,
    contact.team_id,
    contact.email,
    contact.first_name,
    contact.last_name,
    contact.unsubscribed,
    contact.created_at,
    contact.updated_at
FROM contact_segments AS membership
JOIN contacts AS contact
  ON contact.id = membership.contact_id
 AND contact.team_id = membership.team_id
WHERE membership.team_id = sqlc.arg(team_id)
  AND membership.segment_id = sqlc.arg(segment_id)
ORDER BY contact.created_at DESC, contact.id DESC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: DeleteSegment :one
DELETE FROM segments
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;
