-- name: CreateVerificationToken :one
INSERT INTO verification_tokens (
    identifier,
    token_hash,
    expires_at
) VALUES (
    sqlc.arg(identifier),
    sqlc.arg(token_hash),
    sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetVerificationToken :one
SELECT *
FROM verification_tokens
WHERE identifier = sqlc.arg(identifier)
  AND token_hash = sqlc.arg(token_hash)
  AND expires_at > now();

-- name: DeleteVerificationToken :exec
DELETE FROM verification_tokens
WHERE identifier = sqlc.arg(identifier)
  AND token_hash = sqlc.arg(token_hash);

-- name: DeleteVerificationTokensByIdentifier :exec
DELETE FROM verification_tokens
WHERE identifier = sqlc.arg(identifier);

-- name: DeleteExpiredVerificationTokens :exec
DELETE FROM verification_tokens
WHERE expires_at <= now();

-- name: VerifyEmailWithToken :one
WITH consumed AS (
    DELETE FROM verification_tokens
    WHERE verification_tokens.identifier = sqlc.arg(identifier)
      AND verification_tokens.token_hash = sqlc.arg(token_hash)
      AND verification_tokens.expires_at > now()
    RETURNING identifier
)
UPDATE users
SET email_verified = true,
    updated_at = now()
FROM consumed
WHERE lower(users.email) = lower(sqlc.arg(email))
  AND users.email_verified = false
RETURNING users.*;

-- name: ResetPasswordWithToken :one
WITH consumed AS (
    DELETE FROM verification_tokens
    WHERE verification_tokens.identifier = sqlc.arg(identifier)
      AND verification_tokens.token_hash = sqlc.arg(token_hash)
      AND verification_tokens.expires_at > now()
    RETURNING identifier
), updated_user AS (
    UPDATE users
    SET password_hash = sqlc.arg(password_hash),
        credential_version = credential_version + 1,
        security_updated_at = now(),
        updated_at = now()
    FROM consumed
    WHERE lower(users.email) = lower(sqlc.arg(email))
    RETURNING users.*
), revoked_sessions AS (
    UPDATE sessions
    SET revoked_at = now()
    WHERE user_id = (SELECT id FROM updated_user)
      AND revoked_at IS NULL
    RETURNING id
)
SELECT updated_user.*
FROM updated_user;
