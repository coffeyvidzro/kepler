-- name: CreateTeamInvitation :one
INSERT INTO team_invitations (
    team_id,
    email,
    role,
    token_hash,
    invited_by,
    expires_at
)
SELECT
    team.id,
    sqlc.arg(email),
    sqlc.arg(role),
    sqlc.arg(token_hash),
    sqlc.arg(invited_by),
    sqlc.arg(expires_at)
FROM teams AS team
WHERE team.id = sqlc.arg(team_id)
  AND team.status = 'active'
RETURNING *;

-- name: GetTeamInvitationByTokenHash :one
SELECT invitation.*
FROM team_invitations AS invitation
JOIN teams AS team ON team.id = invitation.team_id
WHERE invitation.token_hash = sqlc.arg(token_hash)
  AND invitation.status = 'pending'
  AND invitation.expires_at > now()
  AND team.status = 'active';

-- name: ListPendingTeamInvitations :many
SELECT invitation.*
FROM team_invitations AS invitation
JOIN teams AS team ON team.id = invitation.team_id
WHERE invitation.team_id = sqlc.arg(team_id)
  AND invitation.status = 'pending'
  AND invitation.expires_at > now()
  AND team.status = 'active'
ORDER BY invitation.created_at DESC;

-- name: AcceptTeamInvitation :one
UPDATE team_invitations AS invitation
SET status = 'accepted',
    accepted_at = now(),
    updated_at = now()
FROM teams AS team
WHERE invitation.token_hash = sqlc.arg(token_hash)
  AND invitation.status = 'pending'
  AND invitation.expires_at > now()
  AND team.id = invitation.team_id
  AND team.status = 'active'
RETURNING invitation.*;

-- name: DeclineTeamInvitation :one
UPDATE team_invitations AS invitation
SET status = 'declined',
    declined_at = now(),
    updated_at = now()
FROM teams AS team
WHERE invitation.token_hash = sqlc.arg(token_hash)
  AND invitation.status = 'pending'
  AND invitation.expires_at > now()
  AND team.id = invitation.team_id
  AND team.status = 'active'
RETURNING invitation.*;
