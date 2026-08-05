-- Establish a channel-neutral sender trust plane and delivery-attempt ledger.
--
-- This migration is intentionally additive. Existing sender and message tables
-- remain unchanged so current SQLC queries and runtime paths continue to work
-- while the application migrates to the new control-plane models.

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
        CHECK (
            purpose IS NULL
            OR length(trim(purpose)) > 0
        ),

    CONSTRAINT chk_sender_assets_status
        CHECK (
            status IN (
                'pending',
                'active',
                'degraded',
                'suspended',
                'disabled',
                'failed'
            )
        ),

    CONSTRAINT chk_sender_assets_health_status
        CHECK (health_status IN ('unknown', 'healthy', 'degraded'))
);

-- Email domains are globally exclusive, while tenant-owned SMS identities are
-- scoped to their owning team. Platform identities are globally unique.
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

-- Transitional links make the legacy domain and Sender ID rows traceable during
-- cutover without adding columns to tables that are queried with SELECT *.
CREATE TABLE IF NOT EXISTS sender_asset_legacy_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_asset_id UUID NOT NULL REFERENCES sender_assets(id) ON DELETE CASCADE,
    sender_domain_id UUID UNIQUE REFERENCES sender_domains(id) ON DELETE RESTRICT,
    sender_id UUID UNIQUE REFERENCES sender_ids(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_sender_asset_legacy_links_source
        CHECK (num_nonnulls(sender_domain_id, sender_id) = 1)
);

CREATE INDEX IF NOT EXISTS idx_sender_asset_legacy_links_asset
    ON sender_asset_legacy_links (sender_asset_id);

CREATE TABLE IF NOT EXISTS sender_provider_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_asset_id UUID NOT NULL REFERENCES sender_assets(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_account TEXT NOT NULL DEFAULT 'default',
    region TEXT,
    country_code CHAR(2),
    external_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    provider_status TEXT,
    verified BOOLEAN NOT NULL DEFAULT false,
    health_status TEXT NOT NULL DEFAULT 'unknown',
    verification_data JSONB NOT NULL DEFAULT '{}'::jsonb,
    submitted_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    suspended_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    next_check_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    reconcile_locked_at TIMESTAMPTZ,
    reconcile_locked_by TEXT,
    legacy_sender_domain_id UUID UNIQUE REFERENCES sender_domains(id) ON DELETE RESTRICT,
    legacy_sender_id UUID UNIQUE REFERENCES sender_ids(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_sender_provider_bindings_id_asset
        UNIQUE (id, sender_asset_id),

    CONSTRAINT chk_sender_provider_bindings_provider
        CHECK (
            length(trim(provider)) > 0
            AND length(trim(provider_account)) > 0
        ),

    CONSTRAINT chk_sender_provider_bindings_country
        CHECK (
            country_code IS NULL
            OR country_code ~ '^[A-Z]{2}$'
        ),

    CONSTRAINT chk_sender_provider_bindings_status
        CHECK (
            status IN (
                'pending',
                'active',
                'rejected',
                'suspended',
                'disabled',
                'failed',
                'unknown'
            )
        ),

    CONSTRAINT chk_sender_provider_bindings_health
        CHECK (health_status IN ('unknown', 'healthy', 'degraded')),

    CONSTRAINT chk_sender_provider_bindings_verification_data
        CHECK (jsonb_typeof(verification_data) = 'object'),

    CONSTRAINT chk_sender_provider_bindings_attempts
        CHECK (attempts >= 0),

    CONSTRAINT chk_sender_provider_bindings_error
        CHECK (
            last_error IS NULL
            OR length(trim(last_error)) > 0
        ),

    CONSTRAINT chk_sender_provider_bindings_lock
        CHECK (
            (reconcile_locked_at IS NULL AND reconcile_locked_by IS NULL)
            OR (
                reconcile_locked_at IS NOT NULL
                AND length(trim(reconcile_locked_by)) > 0
            )
        ),

    CONSTRAINT chk_sender_provider_bindings_legacy_source
        CHECK (
            num_nonnulls(legacy_sender_domain_id, legacy_sender_id) <= 1
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_provider_bindings_scope
    ON sender_provider_bindings (
        sender_asset_id,
        lower(provider),
        lower(provider_account),
        COALESCE(lower(region), ''),
        COALESCE(country_code::text, '')
    );

CREATE INDEX IF NOT EXISTS idx_sender_provider_bindings_reconciliation
    ON sender_provider_bindings (next_check_at, created_at)
    WHERE status IN ('pending', 'active', 'unknown')
      AND next_check_at IS NOT NULL;

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

-- Backfill canonical email sender assets.
INSERT INTO sender_assets (
    owner_type,
    team_id,
    channel,
    identity,
    normalized_identity,
    purpose,
    status,
    health_status,
    created_by,
    created_at,
    updated_at
)
SELECT
    'team',
    domain.team_id,
    'email',
    domain.domain,
    lower(trim(domain.domain)),
    'Email sending domain',
    CASE domain.status
        WHEN 'verified' THEN 'active'
        WHEN 'failed' THEN 'failed'
        WHEN 'disabled' THEN 'disabled'
        ELSE 'pending'
    END,
    domain.health_status,
    domain.created_by,
    domain.created_at,
    domain.updated_at
FROM sender_domains AS domain
ON CONFLICT DO NOTHING;

-- Backfill one canonical SMS asset per team and normalized sender name. Country
-- and provider approval remain properties of the provider binding.
WITH grouped_sender_ids AS (
    SELECT
        sender.team_id,
        lower(trim(sender.name)) AS normalized_identity,
        (array_agg(sender.name ORDER BY sender.created_at, sender.id))[1] AS identity,
        (array_agg(sender.purpose ORDER BY sender.created_at, sender.id))[1] AS purpose,
        CASE
            WHEN bool_or(sender.status = 'approved') THEN 'active'
            WHEN bool_or(sender.status = 'pending') THEN 'pending'
            WHEN bool_or(sender.status = 'suspended') THEN 'suspended'
            WHEN bool_or(sender.status = 'rejected') THEN 'failed'
            ELSE 'disabled'
        END AS status,
        CASE
            WHEN bool_or(
                sender.status IN ('suspended', 'rejected')
                OR sender.provider_error IS NOT NULL
            ) THEN 'degraded'
            WHEN bool_or(
                sender.status = 'approved'
                AND sender.provider_whitelisted
            ) THEN 'healthy'
            ELSE 'unknown'
        END AS health_status,
        (array_agg(sender.created_by ORDER BY sender.created_at, sender.id)
            FILTER (WHERE sender.created_by IS NOT NULL))[1] AS created_by,
        min(sender.created_at) AS created_at,
        max(sender.updated_at) AS updated_at
    FROM sender_ids AS sender
    GROUP BY sender.team_id, lower(trim(sender.name))
)
INSERT INTO sender_assets (
    owner_type,
    team_id,
    channel,
    identity,
    normalized_identity,
    purpose,
    status,
    health_status,
    created_by,
    created_at,
    updated_at
)
SELECT
    'team',
    grouped.team_id,
    'sms',
    grouped.identity,
    grouped.normalized_identity,
    grouped.purpose,
    grouped.status,
    grouped.health_status,
    grouped.created_by,
    grouped.created_at,
    grouped.updated_at
FROM grouped_sender_ids AS grouped
ON CONFLICT DO NOTHING;

INSERT INTO sender_asset_legacy_links (
    sender_asset_id,
    sender_domain_id
)
SELECT
    asset.id,
    domain.id
FROM sender_domains AS domain
JOIN sender_assets AS asset
  ON asset.channel = 'email'
 AND asset.team_id = domain.team_id
 AND asset.normalized_identity = lower(trim(domain.domain))
ON CONFLICT DO NOTHING;

INSERT INTO sender_asset_legacy_links (
    sender_asset_id,
    sender_id
)
SELECT
    asset.id,
    sender.id
FROM sender_ids AS sender
JOIN sender_assets AS asset
  ON asset.channel = 'sms'
 AND asset.team_id = sender.team_id
 AND asset.normalized_identity = lower(trim(sender.name))
ON CONFLICT DO NOTHING;

-- Backfill provider bindings for email domains.
INSERT INTO sender_provider_bindings (
    sender_asset_id,
    provider,
    region,
    status,
    provider_status,
    verified,
    health_status,
    verification_data,
    verified_at,
    last_checked_at,
    next_check_at,
    attempts,
    last_error,
    reconcile_locked_at,
    reconcile_locked_by,
    legacy_sender_domain_id,
    created_at,
    updated_at
)
SELECT
    link.sender_asset_id,
    lower(trim(domain.provider)),
    domain.provider_region,
    CASE domain.status
        WHEN 'verified' THEN 'active'
        WHEN 'failed' THEN 'failed'
        WHEN 'disabled' THEN 'disabled'
        ELSE 'pending'
    END,
    domain.status,
    domain.status = 'verified',
    domain.health_status,
    jsonb_build_object(
        'records', domain.verification_records,
        'legacy_source', 'sender_domains'
    ),
    domain.verified_at,
    domain.last_checked_at,
    domain.next_check_at,
    domain.verification_attempts,
    domain.failure_reason,
    domain.reconcile_locked_at,
    domain.reconcile_locked_by,
    domain.id,
    domain.created_at,
    domain.updated_at
FROM sender_domains AS domain
JOIN sender_asset_legacy_links AS link
  ON link.sender_domain_id = domain.id
ON CONFLICT DO NOTHING;

-- Backfill provider bindings for Sender IDs that already have a provider.
INSERT INTO sender_provider_bindings (
    sender_asset_id,
    provider,
    country_code,
    status,
    provider_status,
    verified,
    health_status,
    verification_data,
    submitted_at,
    verified_at,
    rejected_at,
    suspended_at,
    last_checked_at,
    next_check_at,
    attempts,
    last_error,
    reconcile_locked_at,
    reconcile_locked_by,
    legacy_sender_id,
    created_at,
    updated_at
)
SELECT
    link.sender_asset_id,
    lower(trim(sender.provider)),
    sender.country_code,
    CASE sender.status
        WHEN 'approved' THEN 'active'
        WHEN 'rejected' THEN 'rejected'
        WHEN 'suspended' THEN 'suspended'
        WHEN 'inactive' THEN 'disabled'
        ELSE 'pending'
    END,
    sender.provider_status,
    sender.status = 'approved',
    CASE
        WHEN sender.status IN ('suspended', 'rejected')
             OR sender.provider_error IS NOT NULL THEN 'degraded'
        WHEN sender.status = 'approved'
             AND sender.provider_whitelisted THEN 'healthy'
        ELSE 'unknown'
    END,
    jsonb_build_object(
        'whitelisted', sender.provider_whitelisted,
        'purpose', sender.purpose,
        'legacy_source', 'sender_ids'
    ),
    sender.provider_submitted_at,
    sender.approved_at,
    sender.rejected_at,
    sender.suspended_at,
    sender.provider_last_checked_at,
    sender.next_status_check_at,
    sender.provider_attempts,
    sender.provider_error,
    sender.registration_locked_at,
    sender.registration_locked_by,
    sender.id,
    sender.created_at,
    sender.updated_at
FROM sender_ids AS sender
JOIN sender_asset_legacy_links AS link
  ON link.sender_id = sender.id
WHERE NULLIF(trim(sender.provider), '') IS NOT NULL
ON CONFLICT DO NOTHING;

-- Tenant-owned assets receive an explicit self-grant. Platform-owned assets can
-- later be granted to one or more teams without duplicating provider bindings.
INSERT INTO sender_asset_grants (
    team_id,
    sender_asset_id,
    channel,
    status,
    granted_by,
    granted_at,
    created_at,
    updated_at
)
SELECT
    asset.team_id,
    asset.id,
    asset.channel,
    'active',
    asset.created_by,
    asset.created_at,
    asset.created_at,
    asset.updated_at
FROM sender_assets AS asset
WHERE asset.owner_type = 'team'
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS message_delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    email_message_id UUID,
    sms_message_id UUID,
    attempt_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'claimed',
    provider TEXT,
    provider_account TEXT NOT NULL DEFAULT 'default',
    provider_message_id TEXT,
    provider_status TEXT,
    sender_asset_id UUID REFERENCES sender_assets(id) ON DELETE SET NULL,
    sender_provider_binding_id UUID,
    error_code TEXT,
    error_message TEXT,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_started_at TIMESTAMPTZ,
    request_completed_at TIMESTAMPTZ,
    submitted_at TIMESTAMPTZ,
    terminal_at TIMESTAMPTZ,
    next_reconcile_at TIMESTAMPTZ,
    last_reconciled_at TIMESTAMPTZ,
    reconcile_attempts INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    legacy_email_attempt_id UUID UNIQUE
        REFERENCES email_delivery_attempts(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_message_delivery_attempts_email_team
        FOREIGN KEY (email_message_id, team_id)
        REFERENCES email_messages (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_message_delivery_attempts_sms_team
        FOREIGN KEY (sms_message_id, team_id)
        REFERENCES sms_messages (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_message_delivery_attempts_sender_binding
        FOREIGN KEY (sender_provider_binding_id, sender_asset_id)
        REFERENCES sender_provider_bindings (id, sender_asset_id)
        ON DELETE RESTRICT,

    CONSTRAINT chk_message_delivery_attempts_channel
        CHECK (channel IN ('email', 'sms')),

    CONSTRAINT chk_message_delivery_attempts_message
        CHECK (
            (
                channel = 'email'
                AND email_message_id IS NOT NULL
                AND sms_message_id IS NULL
            )
            OR (
                channel = 'sms'
                AND sms_message_id IS NOT NULL
                AND email_message_id IS NULL
            )
        ),

    CONSTRAINT chk_message_delivery_attempts_number
        CHECK (attempt_number > 0),

    CONSTRAINT chk_message_delivery_attempts_status
        CHECK (
            status IN (
                'claimed',
                'request_started',
                'submission_unknown',
                'submitted',
                'accepted',
                'sent',
                'delivered',
                'retryable_failure',
                'permanent_failure',
                'rejected',
                'expired',
                'canceled',
                'unknown'
            )
        ),

    CONSTRAINT chk_message_delivery_attempts_provider
        CHECK (
            provider IS NULL
            OR length(trim(provider)) > 0
        ),

    CONSTRAINT chk_message_delivery_attempts_provider_account
        CHECK (length(trim(provider_account)) > 0),

    CONSTRAINT chk_message_delivery_attempts_sender_reference
        CHECK (
            sender_provider_binding_id IS NULL
            OR sender_asset_id IS NOT NULL
        ),

    CONSTRAINT chk_message_delivery_attempts_reconcile_attempts
        CHECK (reconcile_attempts >= 0),

    CONSTRAINT chk_message_delivery_attempts_metadata
        CHECK (jsonb_typeof(metadata) = 'object'),

    CONSTRAINT chk_message_delivery_attempts_timestamps
        CHECK (
            (request_started_at IS NULL OR request_started_at >= claimed_at)
            AND (
                request_completed_at IS NULL
                OR request_started_at IS NULL
                OR request_completed_at >= request_started_at
            )
            AND (
                submitted_at IS NULL
                OR submitted_at >= claimed_at
            )
            AND (
                terminal_at IS NULL
                OR terminal_at >= claimed_at
            )
        )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_message_delivery_attempts_email_number
    ON message_delivery_attempts (email_message_id, attempt_number)
    WHERE email_message_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_message_delivery_attempts_sms_number
    ON message_delivery_attempts (sms_message_id, attempt_number)
    WHERE sms_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_provider_message
    ON message_delivery_attempts (provider, provider_message_id)
    WHERE provider IS NOT NULL AND provider_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_message_created
    ON message_delivery_attempts (
        channel,
        COALESCE(email_message_id, sms_message_id),
        created_at DESC
    );

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_reconciliation
    ON message_delivery_attempts (next_reconcile_at, created_at)
    WHERE status IN (
        'submission_unknown',
        'submitted',
        'accepted',
        'sent',
        'unknown'
    )
      AND next_reconcile_at IS NOT NULL;

-- Preserve the existing email attempt IDs so the compatibility path can be
-- migrated without an identifier translation table.
INSERT INTO message_delivery_attempts (
    id,
    team_id,
    channel,
    email_message_id,
    attempt_number,
    status,
    provider,
    provider_message_id,
    sender_asset_id,
    sender_provider_binding_id,
    error_code,
    error_message,
    claimed_at,
    request_started_at,
    request_completed_at,
    submitted_at,
    reconcile_attempts,
    metadata,
    legacy_email_attempt_id,
    created_at,
    updated_at
)
SELECT
    attempt.id,
    attempt.team_id,
    'email',
    attempt.email_message_id,
    attempt.attempt_number,
    CASE attempt.status
        WHEN 'failed' THEN 'permanent_failure'
        ELSE attempt.status
    END,
    lower(trim(attempt.provider)),
    attempt.provider_message_id,
    asset_link.sender_asset_id,
    binding.id,
    attempt.error_code,
    attempt.error_message,
    attempt.claimed_at,
    attempt.request_started_at,
    attempt.completed_at,
    CASE
        WHEN attempt.status = 'submitted'
            THEN COALESCE(message.submitted_at, attempt.completed_at)
        ELSE NULL
    END,
    0,
    jsonb_build_object(
        'legacy_source', 'email_delivery_attempts',
        'history_complete', true
    ),
    attempt.id,
    attempt.created_at,
    attempt.updated_at
FROM email_delivery_attempts AS attempt
JOIN email_messages AS message
  ON message.id = attempt.email_message_id
LEFT JOIN sender_asset_legacy_links AS asset_link
  ON asset_link.sender_domain_id = message.sender_domain_id
LEFT JOIN sender_provider_bindings AS binding
  ON binding.legacy_sender_domain_id = message.sender_domain_id
 AND lower(binding.provider) = lower(attempt.provider)
ON CONFLICT DO NOTHING;

-- SMS previously stored only the latest provider state on the message. Create a
-- best-effort first attempt and mark the imported history as incomplete.
INSERT INTO message_delivery_attempts (
    team_id,
    channel,
    sms_message_id,
    attempt_number,
    status,
    provider,
    provider_message_id,
    provider_status,
    sender_asset_id,
    sender_provider_binding_id,
    error_message,
    claimed_at,
    request_started_at,
    request_completed_at,
    submitted_at,
    terminal_at,
    reconcile_attempts,
    metadata,
    created_at,
    updated_at
)
SELECT
    message.team_id,
    'sms',
    message.id,
    1,
    CASE message.status
        WHEN 'queued' THEN 'claimed'
        WHEN 'processing' THEN 'request_started'
        WHEN 'submitted' THEN 'submitted'
        WHEN 'sent' THEN 'sent'
        WHEN 'delivered' THEN 'delivered'
        WHEN 'undelivered' THEN 'permanent_failure'
        WHEN 'failed' THEN 'permanent_failure'
        ELSE message.status
    END,
    NULLIF(lower(trim(message.provider_id)), ''),
    message.provider_message_id,
    message.status,
    asset_link.sender_asset_id,
    binding.id,
    message.error_message,
    message.created_at,
    CASE
        WHEN message.status <> 'queued'
            OR message.provider_id IS NOT NULL
            OR message.provider_message_id IS NOT NULL
        THEN message.created_at
        ELSE NULL
    END,
    CASE
        WHEN message.provider_message_id IS NOT NULL
        THEN COALESCE(message.submitted_at, message.updated_at)
        ELSE NULL
    END,
    message.submitted_at,
    CASE
        WHEN message.status IN (
            'delivered',
            'undelivered',
            'rejected',
            'failed',
            'expired',
            'canceled'
        )
        THEN COALESCE(message.delivered_at, message.updated_at)
        ELSE NULL
    END,
    0,
    jsonb_build_object(
        'legacy_source', 'sms_messages',
        'history_complete', false
    ),
    message.created_at,
    message.updated_at
FROM sms_messages AS message
LEFT JOIN sender_asset_legacy_links AS asset_link
  ON asset_link.sender_id = message.sender_id
LEFT JOIN sender_provider_bindings AS binding
  ON binding.legacy_sender_id = message.sender_id
WHERE message.status <> 'queued'
   OR message.provider_id IS NOT NULL
   OR message.provider_message_id IS NOT NULL
ON CONFLICT DO NOTHING;
