-- Create the canonical sender ID trust infrastructure before message tables.
--
-- sender_assets owns the channel-neutral identity. Provider-, account-, region-,
-- and country-specific trust state lives in sender_provider_bindings. Grants
-- authorize teams to use either team-owned or platform-owned assets.

CREATE TABLE IF NOT EXISTS sender_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type TEXT NOT NULL,
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    identity TEXT NOT NULL,
    normalized_identity TEXT NOT NULL,
    purpose TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    health_status TEXT NOT NULL DEFAULT 'unknown',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_sender_assets_id_channel UNIQUE (id, channel),
    CONSTRAINT chk_sender_assets_owner_type
        CHECK (owner_type IN ('platform', 'team')),
    CONSTRAINT chk_sender_assets_owner_scope
        CHECK (
            (owner_type = 'platform' AND team_id IS NULL)
            OR (owner_type = 'team' AND team_id IS NOT NULL)
        ),
    CONSTRAINT chk_sender_assets_channel
        CHECK (channel IN ('email', 'sms')),
    CONSTRAINT chk_sender_assets_identity
        CHECK (
            length(trim(identity)) > 0
            AND normalized_identity = lower(trim(identity))
        ),
    CONSTRAINT chk_sender_assets_purpose
        CHECK (purpose IS NULL OR length(trim(purpose)) > 0),
    CONSTRAINT chk_sender_assets_status
        CHECK (status IN (
            'pending', 'active', 'degraded', 'suspended', 'disabled', 'failed'
        )),
    CONSTRAINT chk_sender_assets_health_status
        CHECK (health_status IN ('unknown', 'healthy', 'degraded'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_assets_email_identity
    ON sender_assets (normalized_identity)
    WHERE channel = 'email';

CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_assets_team_identity
    ON sender_assets (team_id, channel, normalized_identity)
    WHERE owner_type = 'team';

CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_assets_platform_identity
    ON sender_assets (channel, normalized_identity)
    WHERE owner_type = 'platform';

CREATE INDEX IF NOT EXISTS idx_sender_assets_team_channel_status
    ON sender_assets (team_id, channel, status)
    WHERE team_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sender_assets_channel_health
    ON sender_assets (channel, health_status, status);

CREATE TABLE IF NOT EXISTS sender_provider_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_asset_id UUID NOT NULL REFERENCES sender_assets(id) ON DELETE CASCADE,
    provider TEXT,
    provider_account TEXT NOT NULL DEFAULT 'default',
    region TEXT,
    country_code CHAR(2),
    external_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    provider_status TEXT,
    verified BOOLEAN NOT NULL DEFAULT false,
    provider_whitelisted BOOLEAN NOT NULL DEFAULT false,
    health_status TEXT NOT NULL DEFAULT 'unknown',
    verification_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    submitted_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    suspended_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    next_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts INTEGER NOT NULL DEFAULT 0,
    consecutive_health_failures INTEGER NOT NULL DEFAULT 0,
    last_health_checked_at TIMESTAMPTZ,
    last_health_failure_at TIMESTAMPTZ,
    rejection_reason TEXT,
    last_error TEXT,
    reconcile_locked_at TIMESTAMPTZ,
    reconcile_locked_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_sender_provider_bindings_id_asset
        UNIQUE (id, sender_asset_id),
    CONSTRAINT chk_sender_provider_bindings_provider
        CHECK (
            provider IS NULL
            OR length(trim(provider)) > 0
        ),
    CONSTRAINT chk_sender_provider_bindings_provider_account
        CHECK (length(trim(provider_account)) > 0),
    CONSTRAINT chk_sender_provider_bindings_country
        CHECK (country_code IS NULL OR country_code ~ '^[A-Z]{2}$'),
    CONSTRAINT chk_sender_provider_bindings_status
        CHECK (status IN (
            'pending', 'active', 'rejected', 'suspended',
            'disabled', 'failed', 'unknown'
        )),
    CONSTRAINT chk_sender_provider_bindings_health
        CHECK (health_status IN ('unknown', 'healthy', 'degraded')),
    CONSTRAINT chk_sender_provider_bindings_verification_data
        CHECK (jsonb_typeof(verification_data) = 'object'),
    CONSTRAINT chk_sender_provider_bindings_attempts
        CHECK (attempts >= 0),
    CONSTRAINT chk_sender_provider_bindings_health_failures
        CHECK (consecutive_health_failures >= 0),
    CONSTRAINT chk_sender_provider_bindings_rejection_reason
        CHECK (
            rejection_reason IS NULL
            OR length(trim(rejection_reason)) > 0
        ),
    CONSTRAINT chk_sender_provider_bindings_error
        CHECK (last_error IS NULL OR length(trim(last_error)) > 0),
    CONSTRAINT chk_sender_provider_bindings_lock
        CHECK (
            (reconcile_locked_at IS NULL AND reconcile_locked_by IS NULL)
            OR (
                reconcile_locked_at IS NOT NULL
                AND length(trim(reconcile_locked_by)) > 0
            )
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_provider_bindings_scope
    ON sender_provider_bindings (
        sender_asset_id,
        COALESCE(lower(provider), ''),
        lower(provider_account),
        COALESCE(lower(region), ''),
        COALESCE(country_code::text, '')
    );

CREATE INDEX IF NOT EXISTS idx_sender_provider_bindings_reconciliation
    ON sender_provider_bindings (next_check_at, created_at)
    WHERE status IN ('pending', 'active', 'unknown');

CREATE INDEX IF NOT EXISTS idx_sender_provider_bindings_provider_status
    ON sender_provider_bindings (provider, status, health_status);

CREATE TABLE IF NOT EXISTS sender_asset_grants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    sender_asset_id UUID NOT NULL,
    channel TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    is_default BOOLEAN NOT NULL DEFAULT false,
    scope JSONB NOT NULL DEFAULT '{}'::jsonb,
    granted_by UUID REFERENCES users(id) ON DELETE SET NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_sender_asset_grants_asset_channel
        FOREIGN KEY (sender_asset_id, channel)
        REFERENCES sender_assets (id, channel)
        ON DELETE CASCADE,
    CONSTRAINT uq_sender_asset_grants_team_asset
        UNIQUE (team_id, sender_asset_id),
    CONSTRAINT chk_sender_asset_grants_channel
        CHECK (channel IN ('email', 'sms')),
    CONSTRAINT chk_sender_asset_grants_status
        CHECK (status IN ('active', 'revoked')),
    CONSTRAINT chk_sender_asset_grants_scope
        CHECK (jsonb_typeof(scope) = 'object'),
    CONSTRAINT chk_sender_asset_grants_revocation
        CHECK (
            (status = 'active' AND revoked_at IS NULL)
            OR (status = 'revoked' AND revoked_at IS NOT NULL)
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_asset_grants_default
    ON sender_asset_grants (team_id, channel)
    WHERE status = 'active' AND is_default;

CREATE INDEX IF NOT EXISTS idx_sender_asset_grants_asset_status
    ON sender_asset_grants (sender_asset_id, status);
