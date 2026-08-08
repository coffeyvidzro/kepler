-- Create the first-class email domain aggregate.
--
-- Domains are intentionally independent from SMS Sender IDs and own their
-- channel-specific provider and trust state directly.

CREATE TABLE IF NOT EXISTS domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'aws_ses',
    provider_account TEXT NOT NULL DEFAULT 'default',
    provider_region TEXT NOT NULL,
    provider_external_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    provider_status TEXT,
    open_tracking BOOLEAN NOT NULL DEFAULT true,
    click_tracking BOOLEAN NOT NULL DEFAULT false,
    tracking_subdomain TEXT,
    active_tracking_subdomain TEXT,
    tls_mode TEXT NOT NULL DEFAULT 'opportunistic',
    sending_enabled BOOLEAN NOT NULL DEFAULT true,
    receiving_enabled BOOLEAN NOT NULL DEFAULT false,
    custom_return_path TEXT NOT NULL DEFAULT 'send',
    health_status TEXT NOT NULL DEFAULT 'unknown',
    consecutive_health_failures INTEGER NOT NULL DEFAULT 0,
    failure_reason TEXT,
    last_error TEXT,
    submitted_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    next_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_health_checked_at TIMESTAMPTZ,
    last_health_failure_at TIMESTAMPTZ,
    reconciliation_attempts INTEGER NOT NULL DEFAULT 0,
    reconcile_locked_at TIMESTAMPTZ,
    reconcile_locked_by TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_domains_id_team UNIQUE (id, team_id),
    CONSTRAINT uq_domains_normalized_name UNIQUE (normalized_name),
    CONSTRAINT chk_domains_name CHECK (
        length(trim(name)) > 0
        AND normalized_name = lower(trim(name))
    ),
    CONSTRAINT chk_domains_provider CHECK (length(trim(provider)) > 0),
    CONSTRAINT chk_domains_provider_account CHECK (length(trim(provider_account)) > 0),
    CONSTRAINT chk_domains_provider_region CHECK (length(trim(provider_region)) > 0),
    CONSTRAINT chk_domains_status CHECK (status IN (
        'not_started', 'pending', 'verified', 'partially_verified',
        'partially_failed', 'failed', 'temporary_failure', 'disabled'
    )),
    CONSTRAINT chk_domains_tls_mode CHECK (tls_mode IN ('opportunistic', 'enforced')),
    CONSTRAINT chk_domains_capabilities CHECK (sending_enabled OR receiving_enabled),
    CONSTRAINT chk_domains_health_status CHECK (health_status IN ('unknown', 'healthy', 'degraded')),
    CONSTRAINT chk_domains_health_failures CHECK (consecutive_health_failures >= 0),
    CONSTRAINT chk_domains_reconciliation_attempts CHECK (reconciliation_attempts >= 0),
    CONSTRAINT chk_domains_failure_reason CHECK (
        failure_reason IS NULL OR length(trim(failure_reason)) > 0
    ),
    CONSTRAINT chk_domains_last_error CHECK (
        last_error IS NULL OR length(trim(last_error)) > 0
    ),
    CONSTRAINT chk_domains_reconcile_lock CHECK (
        (reconcile_locked_at IS NULL AND reconcile_locked_by IS NULL)
        OR (
            reconcile_locked_at IS NOT NULL
            AND reconcile_locked_by IS NOT NULL
            AND length(trim(reconcile_locked_by)) > 0
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_domains_team_created
    ON domains (team_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_domains_reconciliation
    ON domains (next_check_at, created_at)
    WHERE status IN ('pending', 'verified', 'temporary_failure')
      AND disabled_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_domains_provider_status
    ON domains (provider, provider_region, status, health_status);

CREATE TABLE IF NOT EXISTS domain_dns_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_id UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    record TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    value TEXT NOT NULL,
    ttl TEXT NOT NULL DEFAULT 'Auto',
    priority INTEGER,
    status TEXT NOT NULL DEFAULT 'pending',
    is_current BOOLEAN NOT NULL DEFAULT true,
    verified_at TIMESTAMPTZ,
    superseded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_domain_dns_records_purpose CHECK (purpose IN (
        'dkim', 'spf', 'mail_from', 'tracking', 'claim'
    )),
    CONSTRAINT chk_domain_dns_records_record CHECK (length(trim(record)) > 0),
    CONSTRAINT chk_domain_dns_records_name CHECK (length(trim(name)) > 0),
    CONSTRAINT chk_domain_dns_records_type CHECK (length(trim(type)) > 0),
    CONSTRAINT chk_domain_dns_records_value CHECK (length(trim(value)) > 0),
    CONSTRAINT chk_domain_dns_records_ttl CHECK (length(trim(ttl)) > 0),
    CONSTRAINT chk_domain_dns_records_status CHECK (status IN (
        'not_started', 'pending', 'verified', 'failed', 'temporary_failure'
    )),
    CONSTRAINT chk_domain_dns_records_priority CHECK (priority IS NULL OR priority >= 0),
    CONSTRAINT chk_domain_dns_records_lifecycle CHECK (
        (is_current AND superseded_at IS NULL)
        OR (NOT is_current AND superseded_at IS NOT NULL)
    ),
    CONSTRAINT uq_domain_dns_records_identity UNIQUE (
        domain_id, purpose, name, type, value
    )
);

CREATE INDEX IF NOT EXISTS idx_domain_dns_records_domain_current
    ON domain_dns_records (domain_id, purpose, created_at)
    WHERE is_current;

CREATE INDEX IF NOT EXISTS idx_domain_dns_records_verification
    ON domain_dns_records (status, updated_at)
    WHERE is_current AND status <> 'verified';

CREATE TABLE IF NOT EXISTS domain_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    target_domain_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    source_domain_id UUID REFERENCES domains(id) ON DELETE SET NULL,
    normalized_name TEXT NOT NULL,
    source_team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    target_team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    provider_region TEXT NOT NULL,
    custom_return_path TEXT NOT NULL DEFAULT 'send',
    open_tracking BOOLEAN NOT NULL DEFAULT true,
    click_tracking BOOLEAN NOT NULL DEFAULT false,
    tracking_subdomain TEXT,
    tls_mode TEXT NOT NULL DEFAULT 'opportunistic',
    sending_enabled BOOLEAN NOT NULL DEFAULT true,
    receiving_enabled BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'pending',
    blocked_reason TEXT,
    failure_reason TEXT,
    record_name TEXT NOT NULL,
    record_value TEXT NOT NULL,
    record_ttl TEXT NOT NULL DEFAULT 'Auto',
    verification_requested_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    reconcile_locked_at TIMESTAMPTZ,
    reconcile_locked_by TEXT,
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_domain_claims_name CHECK (length(trim(normalized_name)) > 0),
    CONSTRAINT chk_domain_claims_teams CHECK (source_team_id <> target_team_id),
    CONSTRAINT chk_domain_claims_region CHECK (length(trim(provider_region)) > 0),
    CONSTRAINT chk_domain_claims_tls_mode CHECK (tls_mode IN ('opportunistic', 'enforced')),
    CONSTRAINT chk_domain_claims_capabilities CHECK (sending_enabled OR receiving_enabled),
    CONSTRAINT chk_domain_claims_status CHECK (status IN (
        'pending', 'verified', 'completed', 'blocked', 'expired',
        'superseded', 'canceled', 'failed'
    )),
    CONSTRAINT chk_domain_claims_blocked_reason CHECK (
        blocked_reason IS NULL OR blocked_reason IN (
            'grace_period', 'recent_owner_activity', 'pending_scheduled_emails'
        )
    ),
    CONSTRAINT chk_domain_claims_failure_reason CHECK (
        failure_reason IS NULL OR length(trim(failure_reason)) > 0
    ),
    CONSTRAINT chk_domain_claims_record CHECK (
        length(trim(record_name)) > 0
        AND length(trim(record_value)) > 0
        AND length(trim(record_ttl)) > 0
    ),
    CONSTRAINT chk_domain_claims_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_domain_claims_reconcile_lock CHECK (
        (reconcile_locked_at IS NULL AND reconcile_locked_by IS NULL)
        OR (
            reconcile_locked_at IS NOT NULL
            AND reconcile_locked_by IS NOT NULL
            AND length(trim(reconcile_locked_by)) > 0
        )
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_domain_claims_active_name
    ON domain_claims (normalized_name)
    WHERE status IN ('pending', 'verified', 'blocked');

CREATE INDEX IF NOT EXISTS idx_domain_claims_target_team_created
    ON domain_claims (target_team_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_domain_claims_reconciliation
    ON domain_claims (verification_requested_at, created_at)
    WHERE status IN ('pending', 'verified', 'blocked')
      AND verification_requested_at IS NOT NULL;
