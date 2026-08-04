CREATE TABLE IF NOT EXISTS sender_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    domain TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'aws_ses',
    provider_region TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    verification_records JSONB NOT NULL DEFAULT '[]'::jsonb,
    failure_reason TEXT,

    last_checked_at TIMESTAMPTZ,
    next_check_at TIMESTAMPTZ NOT NULL DEFAULT (now() + INTERVAL '1 minute'),
    verification_attempts INTEGER NOT NULL DEFAULT 0,
    reconcile_locked_at TIMESTAMPTZ,
    reconcile_locked_by TEXT,
    health_status TEXT NOT NULL DEFAULT 'unknown',
    consecutive_health_failures INTEGER NOT NULL DEFAULT 0,
    last_health_checked_at TIMESTAMPTZ,
    last_health_failure_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_sender_domains_domain_not_empty
        CHECK (length(trim(domain)) > 0),

    CONSTRAINT chk_sender_domains_domain_normalized
        CHECK (domain = lower(trim(domain))),

    CONSTRAINT chk_sender_domains_provider_not_empty
        CHECK (length(trim(provider)) > 0),

    CONSTRAINT chk_sender_domains_provider_region_not_empty
        CHECK (length(trim(provider_region)) > 0),

    CONSTRAINT chk_sender_domains_status
        CHECK (
            status IN (
                'pending',
                'verified',
                'failed',
                'disabled'
            )
        ),

    CONSTRAINT chk_sender_domains_verification_records_array
        CHECK (jsonb_typeof(verification_records) = 'array'),

    CONSTRAINT chk_sender_domains_failure_reason
        CHECK (
            failure_reason IS NULL
            OR length(trim(failure_reason)) > 0
        ),

    CONSTRAINT chk_sender_domains_verification_attempts
        CHECK (verification_attempts >= 0),

    CONSTRAINT chk_sender_domains_reconcile_lock_pair
        CHECK (
            (reconcile_locked_at IS NULL AND reconcile_locked_by IS NULL)
            OR (reconcile_locked_at IS NOT NULL AND length(trim(reconcile_locked_by)) > 0)
        ),

    CONSTRAINT chk_sender_domains_health_status
        CHECK (health_status IN ('unknown', 'healthy', 'degraded')),

    CONSTRAINT chk_sender_domains_health_failures
        CHECK (consecutive_health_failures >= 0)
);

-- A verified domain should belong to only one tenant.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_domains_domain
    ON sender_domains (lower(domain));

CREATE INDEX IF NOT EXISTS idx_sender_domains_team_id
    ON sender_domains (team_id);

CREATE INDEX IF NOT EXISTS idx_sender_domains_team_id_status
    ON sender_domains (team_id, status);

CREATE INDEX IF NOT EXISTS idx_sender_domains_provider_status
    ON sender_domains (provider, status);

CREATE INDEX IF NOT EXISTS idx_sender_domains_reconciliation
    ON sender_domains (next_check_at, created_at)
    WHERE status IN ('pending', 'verified') AND disabled_at IS NULL;
