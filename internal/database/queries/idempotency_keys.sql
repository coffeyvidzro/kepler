-- name: CreateIdempotencyKey :one
INSERT INTO idempotency_keys (
    scope,
    idempotency_key,
    method,
    path,
    request_hash,
    status,
    locked_until,
    expires_at
) VALUES (
    sqlc.arg(scope),
    sqlc.arg(idempotency_key),
    sqlc.arg(method),
    sqlc.arg(path),
    sqlc.arg(request_hash),
    'processing',
    sqlc.arg(locked_until),
    sqlc.arg(expires_at)
)
ON CONFLICT ON CONSTRAINT idempotency_keys_pkey DO NOTHING
RETURNING
    scope,
    idempotency_key,
    method,
    path,
    request_hash,
    status,
    response_status,
    response_body,
    response_content_type,
    response_headers,
    locked_until,
    completed_at,
    expires_at,
    created_at,
    updated_at;

-- name: GetIdempotencyKey :one
SELECT
    scope,
    idempotency_key,
    method,
    path,
    request_hash,
    status,
    response_status,
    response_body,
    response_content_type,
    response_headers,
    locked_until,
    completed_at,
    expires_at,
    created_at,
    updated_at
FROM idempotency_keys
WHERE scope = sqlc.arg(scope)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: CompleteIdempotencyKey :exec
UPDATE idempotency_keys
SET
    status = 'completed',
    response_status = sqlc.arg(response_status),
    response_body = sqlc.arg(response_body),
    response_content_type = sqlc.narg(response_content_type),
    response_headers = sqlc.arg(response_headers),
    completed_at = now(),
    updated_at = now()
WHERE scope = sqlc.arg(scope)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_keys
WHERE scope = sqlc.arg(scope)
  AND idempotency_key = sqlc.arg(idempotency_key);
