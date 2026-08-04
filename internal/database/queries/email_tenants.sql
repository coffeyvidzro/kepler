-- name: CreateEmailTenant :one
INSERT INTO email_tenants (
    team_id,
    provider,
    region,
    external_name,
    suppression_scope,
    reputation_policy
)
VALUES (
    sqlc.arg(team_id),
    sqlc.arg(provider),
    sqlc.arg(region),
    sqlc.arg(external_name),
    sqlc.arg(suppression_scope),
    sqlc.arg(reputation_policy)
)
ON CONFLICT (team_id, provider, region)
DO UPDATE SET updated_at = email_tenants.updated_at
RETURNING *;

-- name: GetEmailTenant :one
SELECT *
FROM email_tenants
WHERE id = sqlc.arg(id);

-- name: GetEmailTenantByTeamProviderRegion :one
SELECT *
FROM email_tenants
WHERE team_id = sqlc.arg(team_id)
  AND provider = sqlc.arg(provider)
  AND region = sqlc.arg(region);

-- name: MarkEmailTenantProvisioning :one
UPDATE email_tenants
SET status = 'provisioning',
    failure_reason = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status IN ('pending', 'failed')
RETURNING *;

-- name: MarkEmailTenantActive :one
UPDATE email_tenants
SET external_id = sqlc.narg(external_id),
    tenant_arn = sqlc.narg(tenant_arn),
    status = 'active',
    failure_reason = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'provisioning'
RETURNING *;

-- name: MarkEmailTenantFailed :one
UPDATE email_tenants
SET status = 'failed',
    failure_reason = sqlc.narg(failure_reason),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status IN ('pending', 'provisioning')
RETURNING *;

-- name: MarkEmailTenantPaused :one
UPDATE email_tenants
SET status = 'paused',
    failure_reason = sqlc.narg(failure_reason),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'active'
RETURNING *;

-- name: MarkEmailTenantDeleting :one
UPDATE email_tenants
SET status = 'deleting',
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status IN ('active', 'paused', 'failed')
RETURNING *;

-- name: DeleteEmailTenant :execrows
DELETE FROM email_tenants
WHERE id = sqlc.arg(id)
  AND status = 'deleting';
