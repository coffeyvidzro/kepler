-- name: CreateOAuthIdentity :one
INSERT INTO oauth_identities (
    user_id,
    provider,
    provider_uid
) VALUES (
    sqlc.arg(user_id),
    sqlc.arg(provider),
    sqlc.arg(provider_uid)
)
RETURNING *;

-- name: GetOAuthIdentity :one
SELECT *
FROM oauth_identities
WHERE provider = sqlc.arg(provider)
  AND provider_uid = sqlc.arg(provider_uid);

-- name: ListOAuthIdentitiesByUserID :many
SELECT *
FROM oauth_identities
WHERE user_id = sqlc.arg(user_id)
ORDER BY created_at ASC;
