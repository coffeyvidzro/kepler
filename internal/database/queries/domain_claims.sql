-- name: CreateDomainClaim :one
INSERT INTO domain_claims (
    target_domain_id,
    source_domain_id,
    normalized_name,
    source_team_id,
    target_team_id,
    provider_region,
    custom_return_path,
    open_tracking,
    click_tracking,
    tracking_subdomain,
    tls_mode,
    sending_enabled,
    receiving_enabled,
    record_name,
    record_value,
    record_ttl,
    expires_at,
    created_by
) VALUES (
    sqlc.arg(target_domain_id),
    sqlc.arg(source_domain_id),
    lower(trim(sqlc.arg(normalized_name))),
    sqlc.arg(source_team_id),
    sqlc.arg(target_team_id),
    lower(trim(sqlc.arg(provider_region))),
    lower(trim(sqlc.arg(custom_return_path))),
    sqlc.arg(open_tracking),
    sqlc.arg(click_tracking),
    sqlc.narg(tracking_subdomain),
    sqlc.arg(tls_mode),
    sqlc.arg(sending_enabled),
    sqlc.arg(receiving_enabled),
    sqlc.arg(record_name),
    sqlc.arg(record_value),
    sqlc.arg(record_ttl),
    sqlc.arg(expires_at),
    sqlc.arg(created_by)
)
RETURNING *;

-- name: GetDomainClaim :one
SELECT claim.*
FROM domain_claims AS claim
WHERE claim.target_domain_id = sqlc.arg(target_domain_id)
  AND claim.target_team_id = sqlc.arg(target_team_id);

-- name: GetDomainClaimByID :one
SELECT claim.*
FROM domain_claims AS claim
WHERE claim.id = sqlc.arg(id);

-- name: RequestDomainClaimVerification :one
UPDATE domain_claims
SET verification_requested_at = now(),
    blocked_reason = NULL,
    failure_reason = NULL,
    updated_at = now()
WHERE target_domain_id = sqlc.arg(target_domain_id)
  AND target_team_id = sqlc.arg(target_team_id)
  AND status IN ('pending', 'blocked')
  AND expires_at > now()
RETURNING *;

-- name: ClaimPendingDomainClaims :many
WITH candidates AS (
    SELECT claim.id
    FROM domain_claims AS claim
    WHERE claim.status IN ('pending', 'verified', 'blocked')
      AND claim.verification_requested_at IS NOT NULL
      AND claim.expires_at > now()
      AND (
          claim.reconcile_locked_at IS NULL
          OR claim.reconcile_locked_at < sqlc.arg(stale_before)
      )
    ORDER BY claim.verification_requested_at, claim.created_at
    FOR UPDATE SKIP LOCKED
    LIMIT sqlc.arg(claim_limit)
), updated AS (
    UPDATE domain_claims AS claim
    SET reconcile_locked_at = now(),
        reconcile_locked_by = trim(sqlc.arg(worker_id)),
        updated_at = now()
    FROM candidates
    WHERE claim.id = candidates.id
    RETURNING claim.*
)
SELECT updated.*
FROM updated
ORDER BY updated.verification_requested_at, updated.created_at;

-- name: MarkDomainClaimVerified :one
UPDATE domain_claims
SET status = 'verified',
    verified_at = COALESCE(verified_at, now()),
    blocked_reason = NULL,
    failure_reason = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: MarkDomainClaimBlocked :one
UPDATE domain_claims
SET status = 'blocked',
    blocked_reason = sqlc.arg(blocked_reason),
    failure_reason = NULL,
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: MarkDomainClaimFailed :one
UPDATE domain_claims
SET status = 'failed',
    failure_reason = sqlc.arg(failure_reason),
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: MarkDomainClaimCompleted :one
UPDATE domain_claims
SET status = 'completed',
    completed_at = COALESCE(completed_at, now()),
    blocked_reason = NULL,
    failure_reason = NULL,
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'verified'
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;

-- name: CancelDomainClaim :one
UPDATE domain_claims
SET status = 'canceled',
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE target_domain_id = sqlc.arg(target_domain_id)
  AND target_team_id = sqlc.arg(target_team_id)
  AND status IN ('pending', 'verified', 'blocked')
RETURNING *;

-- name: ExpireDomainClaims :many
UPDATE domain_claims
SET status = 'expired',
    reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE status IN ('pending', 'verified', 'blocked')
  AND expires_at <= now()
RETURNING *;

-- name: DomainHasPendingScheduledEmails :one
SELECT EXISTS (
    SELECT 1
    FROM email_messages AS message
    WHERE message.sender_domain_id = sqlc.arg(domain_id)
      AND message.status = 'queued'
      AND message.scheduled_at IS NOT NULL
      AND message.scheduled_at > now()
);


-- name: ReleaseDomainClaim :one
UPDATE domain_claims
SET reconcile_locked_at = NULL,
    reconcile_locked_by = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND reconcile_locked_by = trim(sqlc.arg(worker_id))
RETURNING *;
