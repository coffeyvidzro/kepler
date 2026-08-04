-- name: CreateUser :one
INSERT INTO users (
    name,
    email,
    password_hash
) VALUES (
    sqlc.arg(name),
    sqlc.arg(email),
    sqlc.narg(password_hash)
)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = sqlc.arg(id);

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = sqlc.arg(email);

-- name: UpdateUserProfile :one
UPDATE users
SET name = sqlc.arg(name),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUserPassword :one
UPDATE users
SET password_hash = sqlc.narg(password_hash),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateUserEmail :one
UPDATE users
SET email = sqlc.arg(email),
    email_verified = false,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = sqlc.arg(id);

-- name: MarkUserEmailVerifiedByEmail :one
UPDATE users
SET email_verified = true,
    updated_at = now()
WHERE email = sqlc.arg(email)
RETURNING *;

-- name: UpdateUserPasswordByEmail :one
UPDATE users
SET password_hash = sqlc.narg(password_hash),
    updated_at = now()
WHERE email = sqlc.arg(email)
RETURNING *;
