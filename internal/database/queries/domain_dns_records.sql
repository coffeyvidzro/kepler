-- name: CreateDomainDNSRecord :one
INSERT INTO domain_dns_records (
    domain_id,
    purpose,
    record,
    name,
    type,
    value,
    ttl,
    priority,
    status,
    is_current,
    verified_at
) VALUES (
    sqlc.arg(domain_id),
    lower(trim(sqlc.arg(purpose))),
    sqlc.arg(record),
    sqlc.arg(name),
    sqlc.arg(type),
    sqlc.arg(value),
    sqlc.arg(ttl),
    sqlc.narg(priority),
    sqlc.arg(status),
    true,
    CASE WHEN sqlc.arg(status) = 'verified' THEN now() ELSE NULL END
)
ON CONFLICT (domain_id, purpose, name, type, value)
DO UPDATE SET record = EXCLUDED.record,
              ttl = EXCLUDED.ttl,
              priority = EXCLUDED.priority,
              status = EXCLUDED.status,
              is_current = true,
              verified_at = CASE
                  WHEN EXCLUDED.status = 'verified'
                      THEN COALESCE(domain_dns_records.verified_at, now())
                  ELSE domain_dns_records.verified_at
              END,
              superseded_at = NULL,
              updated_at = now()
RETURNING *;

-- name: ListDomainDNSRecords :many
SELECT dns_record.*
FROM domain_dns_records AS dns_record
WHERE dns_record.domain_id = sqlc.arg(domain_id)
  AND dns_record.is_current
ORDER BY dns_record.created_at, dns_record.id;

-- name: ListAllDomainDNSRecords :many
SELECT dns_record.*
FROM domain_dns_records AS dns_record
WHERE dns_record.domain_id = sqlc.arg(domain_id)
ORDER BY dns_record.created_at, dns_record.id;

-- name: DeleteCurrentDomainDNSRecords :exec
DELETE FROM domain_dns_records
WHERE domain_id = sqlc.arg(domain_id)
  AND is_current
  AND purpose <> 'tracking';

-- name: SupersedeDomainTrackingRecords :exec
UPDATE domain_dns_records
SET is_current = false,
    superseded_at = now(),
    updated_at = now()
WHERE domain_id = sqlc.arg(domain_id)
  AND purpose = 'tracking'
  AND is_current;

-- name: DeleteDomainDNSRecords :exec
DELETE FROM domain_dns_records
WHERE domain_id = sqlc.arg(domain_id);
