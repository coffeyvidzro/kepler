-- Create the canonical SMS Sender ID aggregate before message tables.
--
-- A sender_ids row owns the team scope, provider registration state, health,
-- reconciliation lifecycle, and public Sender ID status directly.

CREATE TABLE IF NOT EXISTS sender_ids (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,

    name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    country_code CHAR(2) NOT NULL,
    purpose TEXT,

    provider TEXT,
    provider_status TEXT,
    provider_whitelisted BOOLEAN NOT NULL DEFAULT false,

    status TEXT NOT NULL DEFAULT 'pending',
    health_status TEXT NOT NULL DEFAULT 'unknown',

    submitted_at TIMESTAMPTZ,
    approved_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    suspended_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,

    next_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_checked_at TIMESTAMPTZ,

    reconciliation_attempts INTEGER NOT NULL DEFAULT 0,
    consecutive_health_failures INTEGER NOT NULL DEFAULT 0,
    last_health_checked_at TIMESTAMPTZ,
    last_health_failure_at TIMESTAMPTZ,

    rejection_reason TEXT,
    last_error TEXT,

    reconcile_locked_at TIMESTAMPTZ,
    reconcile_locked_by TEXT,

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_sender_ids_name
        CHECK (
            length(trim(name)) > 0
            AND normalized_name = lower(trim(name))
        ),
    CONSTRAINT chk_sender_ids_country_code
        CHECK (country_code ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_sender_ids_purpose
        CHECK (purpose IS NULL OR length(trim(purpose)) > 0),
    CONSTRAINT chk_sender_ids_provider
        CHECK (provider IS NULL OR length(trim(provider)) > 0),
    CONSTRAINT chk_sender_ids_status
        CHECK (status IN (
            'pending', 'approved', 'rejected', 'suspended', 'inactive'
        )),
    CONSTRAINT chk_sender_ids_health_status
        CHECK (health_status IN ('unknown', 'healthy', 'degraded')),
    CONSTRAINT chk_sender_ids_reconciliation_attempts
        CHECK (reconciliation_attempts >= 0),
    CONSTRAINT chk_sender_ids_health_failures
        CHECK (consecutive_health_failures >= 0),
    CONSTRAINT chk_sender_ids_rejection_reason
        CHECK (
            rejection_reason IS NULL
            OR length(trim(rejection_reason)) > 0
        ),
    CONSTRAINT chk_sender_ids_last_error
        CHECK (last_error IS NULL OR length(trim(last_error)) > 0),
    CONSTRAINT chk_sender_ids_reconcile_lock
        CHECK (
            (reconcile_locked_at IS NULL AND reconcile_locked_by IS NULL)
            OR (
                reconcile_locked_at IS NOT NULL
                AND length(trim(reconcile_locked_by)) > 0
            )
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_ids_team_name_country_provider
    ON sender_ids (
        team_id,
        normalized_name,
        country_code,
        COALESCE(lower(provider), '')
    );

CREATE INDEX IF NOT EXISTS idx_sender_ids_team_status
    ON sender_ids (team_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_sender_ids_provider_status
    ON sender_ids (provider, country_code, status, health_status);

CREATE INDEX IF NOT EXISTS idx_sender_ids_reconciliation
    ON sender_ids (next_check_at, created_at)
    WHERE status IN ('pending', 'approved');
