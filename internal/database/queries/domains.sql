-- name: CreateDomain :one
INSERT INTO domains (
    team_id,
    name,
    normalized_name,
    provider,
    provider_account,
    provider_region,
    custom_return_path,
    open_tracking,
    click_tracking,
    tracking_subdomain,
    tls_mode,
    sending_enabled,
    receiving_enabled,
    created_by,
    submitted_at,
    next_check_at
)
SELECT team.id,
       sqlc.arg(name),
       lower(trim(sqlc.arg(name))),
       lower(trim(sqlc.arg(provider))),
       lower(trim(sqlc.arg(provider_account))),
       lower(trim(sqlc.arg(provider_region))),
       lower(trim(sqlc.arg(custom_return_path))),
       sqlc.arg(open_tracking),
       sqlc.arg(click_tracking),
       sqlc.narg(tracking_subdomain),
       sqlc.arg(tls_mode),
       sqlc.arg(sending_enabled),
       sqlc.arg(receiving_enabled),
       sqlc.arg(created_by),
       now(),
       now() + interval '1 minute'
FROM teams AS team
WHERE team.id = sqlc.arg(team_id)
  AND team.status = 'active'
RETURNING *;

-- name: CreateClaimedDomain :one
INSERT INTO domains (
    id,
    team_id,
    name,
    normalized_name,
    provider,
    provider_account,
    provider_region,
    status,
    provider_status,
    open_tracking,
    click_tracking,
    tracking_subdomain,
    tls_mode,
    sending_enabled,
    receiving_enabled,
    custom_return_path,
    health_status,
    submitted_at,
    next_check_at,
    created_by
) VALUES (
    sqlc.arg(id),
    sqlc.arg(team_id),
    sqlc.arg(name),
    lower(trim(sqlc.arg(name))),
    lower(trim(sqlc.arg(provider))),
    lower(trim(sqlc.arg(provider_account))),
    lower(trim(sqlc.arg(provider_region))),
    'pending',
    'pending',
    sqlc.arg(open_tracking),
    sqlc.arg(click_tracking),
    sqlc.narg(tracking_subdomain),
    sqlc.arg(tls_mode),
    sqlc.arg(sending_enabled),
    sqlc.arg(receiving_enabled),
    lower(trim(sqlc.arg(custom_return_path))),
    'unknown',
    now(),
    now() + interval '1 minute',
    sqlc.narg(created_by)
)
RETURNING *;

-- name: ListDomains :many
SELECT domain_record.*
FROM domains AS domain_record
WHERE domain_record.team_id = sqlc.arg(team_id)
ORDER BY domain_record.created_at DESC, domain_record.id DESC;

-- name: GetDomain :one
SELECT domain_record.*
FROM domains AS domain_record
WHERE domain_record.id = sqlc.arg(id)
  AND domain_record.team_id = sqlc.arg(team_id);

-- name: GetDomainByID :one
SELECT domain_record.*
FROM domains AS domain_record
WHERE domain_record.id = sqlc.arg(id);

-- name: GetDomainByName :one
SELECT domain_record.*
FROM domains AS domain_record
WHERE domain_record.normalized_name = lower(trim(sqlc.arg(name)))
  AND domain_record.team_id = sqlc.arg(team_id);

-- name: GetDomainByNameForClaim :one
SELECT domain_record.*
FROM domains AS domain_record
WHERE domain_record.normalized_name = lower(trim(sqlc.arg(name)))
FOR SHARE;

-- name: UpdateDomainConfiguration :one
UPDATE domains
SET open_tracking = sqlc.arg(open_tracking),
    click_tracking = sqlc.arg(click_tracking),
    tracking_subdomain = sqlc.narg(tracking_subdomain),
    tls_mode = sqlc.arg(tls_mode),
    sending_enabled = sqlc.arg(sending_enabled),
    receiving_enabled = sqlc.arg(receiving_enabled),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND disabled_at IS NULL
RETURNING *;

-- name: UpdateDomainVerification :one
UPDATE domains
SET status = sqlc.arg(status),
    provider_status = sqlc.arg(status),
    failure_reason = CASE
        WHEN sqlc.arg(status) IN ('failed', 'partially_failed', 'temporary_failure')
            THEN sqlc.narg(failure_reason)
        ELSE NULL
    END,
    last_error = sqlc.narg(failure_reason),
    last_checked_at = now(),
    health_status = CASE
        WHEN sqlc.arg(status) = 'verified' THEN 'healthy'
        ELSE health_status
    END,
    consecutive_health_failures = CASE
        WHEN sqlc.arg(status) = 'verified' THEN 0
        ELSE consecutive_health_failures
    END,
    last_health_checked_at = CASE
        WHEN sqlc.arg(status) = 'verified' THEN now()
        ELSE last_health_checked_at
    END,
    verified_at = CASE
        WHEN sqlc.arg(status) = 'verified' THEN COALESCE(verified_at, now())
        ELSE verified_at
    END,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: ClaimPendingDomainReconciliations :many
WITH candidates AS (
    SELECT domain_record.id
    FROM domains AS domain_record
    WHERE domain_record.status IN ('pending', 'verified', 'temporary_failure')
      AND domain_record.disabled_at IS NULL
      AND domain_record.next_check_at <= now()
      AND (
          domain_record.reconcile_locked_at IS NULL
          OR domain_record.reconcile_locked_at < sqlc.arg(stale_before)
      )
    ORDER BY domain_record.next_check_at, domain_record.created_at
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(claim_limit)
), updated AS (
    UPDATE domains AS domain_record
    SET reconcile_locked_at = now(),
        reconcile_locked_by = trim(sqlc.arg(worker_id)),
        reconciliation_attempts = domain_record.reconciliation_attempts + 1,
        updated_at = now()
    FROM candidates
    WHERE domain_record.id = candidates.id
    RETURNING domain_record.*
)
SELECT updated.*
FROM updated
ORDER BY updated.next_check_at, updated.created_at;

-- name: CompleteDomainReconciliation :one
UPDATE domains
SET status = sqlc.arg(status),
    provider_status = sqlc.arg(status),
    failure_reason = CASE
        WHEN sqlc.arg(status) IN ('failed', 'partially_failed', 'temporary_failure')
            THEN sqlc.narg(failure_reason)
        ELSE NULL
    END,
    last_error = sqlc.narg(failure_reason),
    last_checked_at = now(),
    health_status = CASE
        WHEN sqlc.arg(status) = 'verified' THEN 'healthy'
        ELSE health_status
    END,
    consecutive_health_failures = CASE
        WHEN sqlc.arg(status) = 'verified' THEN 0
        ELSE consecutive_health_failures
    END,
    last_health_checked_at = CASE
        WHEN sqlc.arg(status) = 'verified' THEN now()
        ELSE last_health_checked_at
    END,
    verified_at = CASE
        WHEN sqlc.arg(status) = 'verified' THEN COALESCE(verified_at, now())
        ELSE verified_at
    END,
    next_check_at = sqlc.arg(next_check_at),
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: RecordDomainReconciliationFailure :one
UPDATE domains
SET last_error = sqlc.arg(last_error),
    last_checked_at = now(),
    next_check_at = sqlc.arg(next_check_at),
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: CompleteDomainHealthCheck :one
UPDATE domains
SET health_status = 'healthy',
    consecutive_health_failures = 0,
    last_error = NULL,
    last_checked_at = now(),
    last_health_checked_at = now(),
    next_check_at = sqlc.arg(next_check_at),
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'verified'
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: RecordDomainHealthFailure :one
UPDATE domains
SET health_status = CASE
        WHEN consecutive_health_failures + 1 >= sqlc.arg(failure_threshold)
            THEN 'degraded'
        ELSE health_status
    END,
    consecutive_health_failures = consecutive_health_failures + 1,
    last_error = sqlc.arg(last_error),
    last_checked_at = now(),
    last_health_checked_at = now(),
    last_health_failure_at = now(),
    next_check_at = sqlc.arg(next_check_at),
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'verified'
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: UpdateDomainManualHealthCheck :one
UPDATE domains
SET health_status = CASE
        WHEN sqlc.narg(failure_reason)::text IS NULL THEN 'healthy'
        WHEN consecutive_health_failures + 1 >= sqlc.arg(failure_threshold)
            THEN 'degraded'
        ELSE health_status
    END,
    consecutive_health_failures = CASE
        WHEN sqlc.narg(failure_reason)::text IS NULL THEN 0
        ELSE consecutive_health_failures + 1
    END,
    last_error = sqlc.narg(failure_reason),
    last_checked_at = now(),
    last_health_checked_at = now(),
    last_health_failure_at = CASE
        WHEN sqlc.narg(failure_reason)::text IS NULL THEN last_health_failure_at
        ELSE now()
    END,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
  AND status = 'verified'
RETURNING *;

-- name: DisableDomain :one
UPDATE domains
SET status = 'disabled',
    disabled_at = COALESCE(disabled_at, now()),
    next_check_at = now(),
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: DeleteDomain :one
DELETE FROM domains
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)
RETURNING *;

-- name: DeleteDisabledDomainIfUnreferenced :one
DELETE FROM domains AS domain_record
WHERE domain_record.id = sqlc.arg(id)
  AND domain_record.team_id = sqlc.arg(team_id)
  AND domain_record.status = 'disabled'
  AND NOT EXISTS (
      SELECT 1
      FROM email_messages AS message
      WHERE message.sender_domain_id = domain_record.id
  )
RETURNING domain_record.*;


-- name: DeleteDomainForClaim :one
DELETE FROM domains
WHERE id = sqlc.arg(id)
RETURNING *;
