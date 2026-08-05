-- Establish the canonical sender trust plane before message tables are created.
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

-- Compatibility views keep the existing application surface operational while
-- canonical storage lives only in the trust-plane tables. The view row ID is the
-- provider-binding ID, which is the country/region-specific registration.
CREATE OR REPLACE VIEW sender_ids AS
SELECT
    binding.id,
    asset.team_id,
    asset.identity::VARCHAR(11) AS name,
    binding.country_code::TEXT AS country_code,
    COALESCE(asset.purpose, '') AS purpose,
    CASE binding.status
        WHEN 'active' THEN 'approved'
        WHEN 'disabled' THEN 'inactive'
        WHEN 'failed' THEN 'rejected'
        ELSE binding.status
    END AS status,
    binding.provider,
    binding.provider_status,
    binding.provider_whitelisted,
    binding.submitted_at AS provider_submitted_at,
    binding.last_checked_at AS provider_last_checked_at,
    binding.next_check_at AS next_status_check_at,
    binding.attempts AS provider_attempts,
    binding.last_error AS provider_error,
    binding.reconcile_locked_at AS registration_locked_at,
    binding.reconcile_locked_by AS registration_locked_by,
    binding.rejection_reason,
    binding.verified_at AS approved_at,
    binding.rejected_at,
    binding.suspended_at,
    asset.created_by,
    binding.created_at,
    binding.updated_at
FROM sender_provider_bindings AS binding
JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
WHERE asset.channel = 'sms';

CREATE OR REPLACE FUNCTION write_sender_ids_compatibility()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    asset_id UUID;
    binding_row sender_provider_bindings%ROWTYPE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        SELECT asset.id
        INTO asset_id
        FROM sender_assets AS asset
        WHERE asset.owner_type = 'team'
          AND asset.team_id = NEW.team_id
          AND asset.channel = 'sms'
          AND asset.normalized_identity = lower(trim(NEW.name));

        IF asset_id IS NULL THEN
            INSERT INTO sender_assets (
                owner_type, team_id, channel, identity, normalized_identity,
                purpose, status, health_status, created_by
            ) VALUES (
                'team', NEW.team_id, 'sms', NEW.name, lower(trim(NEW.name)),
                NULLIF(trim(NEW.purpose), ''),
                CASE NEW.status
                    WHEN 'approved' THEN 'active'
                    WHEN 'rejected' THEN 'failed'
                    WHEN 'inactive' THEN 'disabled'
                    ELSE COALESCE(NEW.status, 'pending')
                END,
                CASE
                    WHEN NEW.status = 'approved' AND COALESCE(NEW.provider_whitelisted, false)
                        THEN 'healthy'
                    WHEN NEW.status IN ('rejected', 'suspended') OR NEW.provider_error IS NOT NULL
                        THEN 'degraded'
                    ELSE 'unknown'
                END,
                NEW.created_by
            )
            RETURNING id INTO asset_id;

            INSERT INTO sender_asset_grants (
                team_id, sender_asset_id, channel, granted_by
            ) VALUES (
                NEW.team_id, asset_id, 'sms', NEW.created_by
            );
        END IF;

        INSERT INTO sender_provider_bindings (
            sender_asset_id, provider, country_code, status, provider_status,
            verified, provider_whitelisted, health_status, submitted_at,
            verified_at, rejected_at, suspended_at, next_check_at, attempts,
            rejection_reason, last_error, reconcile_locked_at,
            reconcile_locked_by, created_at, updated_at
        ) VALUES (
            asset_id, NULLIF(lower(trim(NEW.provider)), ''), NEW.country_code,
            CASE NEW.status
                WHEN 'approved' THEN 'active'
                WHEN 'rejected' THEN 'rejected'
                WHEN 'inactive' THEN 'disabled'
                ELSE COALESCE(NEW.status, 'pending')
            END,
            NEW.provider_status,
            NEW.status = 'approved',
            COALESCE(NEW.provider_whitelisted, false),
            CASE
                WHEN NEW.status = 'approved' AND COALESCE(NEW.provider_whitelisted, false)
                    THEN 'healthy'
                WHEN NEW.status IN ('rejected', 'suspended') OR NEW.provider_error IS NOT NULL
                    THEN 'degraded'
                ELSE 'unknown'
            END,
            NEW.provider_submitted_at,
            NEW.approved_at,
            NEW.rejected_at,
            NEW.suspended_at,
            COALESCE(NEW.next_status_check_at, now()),
            COALESCE(NEW.provider_attempts, 0),
            NEW.rejection_reason,
            NEW.provider_error,
            NEW.registration_locked_at,
            NEW.registration_locked_by,
            COALESCE(NEW.created_at, now()),
            COALESCE(NEW.updated_at, now())
        )
        RETURNING * INTO binding_row;

        NEW.id := binding_row.id;
        NEW.created_at := binding_row.created_at;
        NEW.updated_at := binding_row.updated_at;
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        SELECT binding.sender_asset_id
        INTO asset_id
        FROM sender_provider_bindings AS binding
        WHERE binding.id = OLD.id;

        UPDATE sender_assets
        SET identity = NEW.name,
            normalized_identity = lower(trim(NEW.name)),
            purpose = NULLIF(trim(NEW.purpose), ''),
            status = CASE NEW.status
                WHEN 'approved' THEN 'active'
                WHEN 'rejected' THEN 'failed'
                WHEN 'inactive' THEN 'disabled'
                ELSE NEW.status
            END,
            health_status = CASE
                WHEN NEW.status = 'approved' AND NEW.provider_whitelisted THEN 'healthy'
                WHEN NEW.status IN ('rejected', 'suspended') OR NEW.provider_error IS NOT NULL THEN 'degraded'
                ELSE health_status
            END,
            updated_at = now()
        WHERE id = asset_id;

        UPDATE sender_provider_bindings
        SET provider = NULLIF(lower(trim(NEW.provider)), ''),
            country_code = NEW.country_code,
            status = CASE NEW.status
                WHEN 'approved' THEN 'active'
                WHEN 'rejected' THEN 'rejected'
                WHEN 'inactive' THEN 'disabled'
                ELSE NEW.status
            END,
            provider_status = NEW.provider_status,
            verified = NEW.status = 'approved',
            provider_whitelisted = NEW.provider_whitelisted,
            health_status = CASE
                WHEN NEW.status = 'approved' AND NEW.provider_whitelisted THEN 'healthy'
                WHEN NEW.status IN ('rejected', 'suspended') OR NEW.provider_error IS NOT NULL THEN 'degraded'
                ELSE health_status
            END,
            submitted_at = NEW.provider_submitted_at,
            verified_at = NEW.approved_at,
            rejected_at = NEW.rejected_at,
            suspended_at = NEW.suspended_at,
            next_check_at = NEW.next_status_check_at,
            attempts = NEW.provider_attempts,
            rejection_reason = NEW.rejection_reason,
            last_error = NEW.provider_error,
            reconcile_locked_at = NEW.registration_locked_at,
            reconcile_locked_by = NEW.registration_locked_by,
            updated_at = now()
        WHERE id = OLD.id
        RETURNING * INTO binding_row;

        NEW.updated_at := binding_row.updated_at;
        RETURN NEW;
    ELSE
        SELECT binding.sender_asset_id
        INTO asset_id
        FROM sender_provider_bindings AS binding
        WHERE binding.id = OLD.id;

        DELETE FROM sender_provider_bindings WHERE id = OLD.id;
        DELETE FROM sender_assets AS asset
        WHERE asset.id = asset_id
          AND NOT EXISTS (
              SELECT 1
              FROM sender_provider_bindings AS binding
              WHERE binding.sender_asset_id = asset.id
          );
        RETURN OLD;
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_write_sender_ids_compatibility ON sender_ids;
CREATE TRIGGER trg_write_sender_ids_compatibility
INSTEAD OF INSERT OR UPDATE OR DELETE ON sender_ids
FOR EACH ROW
EXECUTE FUNCTION write_sender_ids_compatibility();

CREATE OR REPLACE VIEW sender_domains AS
SELECT
    binding.id,
    asset.team_id,
    asset.normalized_identity AS domain,
    CASE binding.provider WHEN 'ses' THEN 'aws_ses' ELSE binding.provider END AS provider,
    COALESCE(binding.region, '') AS provider_region,
    CASE binding.status
        WHEN 'active' THEN 'verified'
        WHEN 'rejected' THEN 'failed'
        ELSE binding.status
    END AS status,
    COALESCE(binding.verification_data -> 'records', '[]'::jsonb) AS verification_records,
    COALESCE(binding.rejection_reason, binding.last_error) AS failure_reason,
    binding.last_checked_at,
    binding.next_check_at,
    binding.attempts AS verification_attempts,
    binding.reconcile_locked_at,
    binding.reconcile_locked_by,
    binding.health_status,
    binding.consecutive_health_failures,
    binding.last_health_checked_at,
    binding.last_health_failure_at,
    binding.verified_at,
    binding.disabled_at,
    asset.created_by,
    binding.created_at,
    binding.updated_at
FROM sender_provider_bindings AS binding
JOIN sender_assets AS asset ON asset.id = binding.sender_asset_id
WHERE asset.channel = 'email';

CREATE OR REPLACE FUNCTION write_sender_domains_compatibility()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    asset_id UUID;
    binding_row sender_provider_bindings%ROWTYPE;
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO sender_assets (
            owner_type, team_id, channel, identity, normalized_identity,
            purpose, status, health_status, created_by
        ) VALUES (
            'team', NEW.team_id, 'email', lower(trim(NEW.domain)),
            lower(trim(NEW.domain)), 'Email sending domain',
            CASE NEW.status
                WHEN 'verified' THEN 'active'
                WHEN 'failed' THEN 'failed'
                ELSE COALESCE(NEW.status, 'pending')
            END,
            COALESCE(NEW.health_status, 'unknown'),
            NEW.created_by
        )
        RETURNING id INTO asset_id;

        INSERT INTO sender_asset_grants (
            team_id, sender_asset_id, channel, granted_by
        ) VALUES (
            NEW.team_id, asset_id, 'email', NEW.created_by
        );

        INSERT INTO sender_provider_bindings (
            sender_asset_id, provider, region, status, provider_status,
            verified, health_status, verification_data, verified_at,
            disabled_at, last_checked_at, next_check_at, attempts,
            consecutive_health_failures, last_health_checked_at,
            last_health_failure_at, rejection_reason, last_error,
            reconcile_locked_at, reconcile_locked_by, created_at, updated_at
        ) VALUES (
            asset_id,
            CASE lower(trim(NEW.provider)) WHEN 'aws_ses' THEN 'ses' ELSE lower(trim(NEW.provider)) END,
            NEW.provider_region,
            CASE NEW.status
                WHEN 'verified' THEN 'active'
                WHEN 'failed' THEN 'failed'
                ELSE COALESCE(NEW.status, 'pending')
            END,
            NEW.status,
            NEW.status = 'verified',
            COALESCE(NEW.health_status, 'unknown'),
            jsonb_build_object('records', COALESCE(NEW.verification_records, '[]'::jsonb)),
            NEW.verified_at,
            NEW.disabled_at,
            NEW.last_checked_at,
            COALESCE(NEW.next_check_at, now() + INTERVAL '1 minute'),
            COALESCE(NEW.verification_attempts, 0),
            COALESCE(NEW.consecutive_health_failures, 0),
            NEW.last_health_checked_at,
            NEW.last_health_failure_at,
            CASE WHEN NEW.status = 'failed' THEN NEW.failure_reason ELSE NULL END,
            NEW.failure_reason,
            NEW.reconcile_locked_at,
            NEW.reconcile_locked_by,
            COALESCE(NEW.created_at, now()),
            COALESCE(NEW.updated_at, now())
        )
        RETURNING * INTO binding_row;

        NEW.id := binding_row.id;
        NEW.provider := CASE binding_row.provider WHEN 'ses' THEN 'aws_ses' ELSE binding_row.provider END;
        NEW.created_at := binding_row.created_at;
        NEW.updated_at := binding_row.updated_at;
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        SELECT binding.sender_asset_id
        INTO asset_id
        FROM sender_provider_bindings AS binding
        WHERE binding.id = OLD.id;

        UPDATE sender_assets
        SET identity = lower(trim(NEW.domain)),
            normalized_identity = lower(trim(NEW.domain)),
            status = CASE NEW.status
                WHEN 'verified' THEN 'active'
                WHEN 'failed' THEN 'failed'
                ELSE NEW.status
            END,
            health_status = NEW.health_status,
            updated_at = now()
        WHERE id = asset_id;

        UPDATE sender_provider_bindings
        SET provider = CASE lower(trim(NEW.provider)) WHEN 'aws_ses' THEN 'ses' ELSE lower(trim(NEW.provider)) END,
            region = NEW.provider_region,
            status = CASE NEW.status
                WHEN 'verified' THEN 'active'
                WHEN 'failed' THEN 'failed'
                ELSE NEW.status
            END,
            provider_status = NEW.status,
            verified = NEW.status = 'verified',
            health_status = NEW.health_status,
            verification_data = jsonb_set(
                verification_data,
                '{records}',
                COALESCE(NEW.verification_records, '[]'::jsonb),
                true
            ),
            verified_at = NEW.verified_at,
            disabled_at = NEW.disabled_at,
            last_checked_at = NEW.last_checked_at,
            next_check_at = NEW.next_check_at,
            attempts = NEW.verification_attempts,
            consecutive_health_failures = NEW.consecutive_health_failures,
            last_health_checked_at = NEW.last_health_checked_at,
            last_health_failure_at = NEW.last_health_failure_at,
            rejection_reason = CASE WHEN NEW.status = 'failed' THEN NEW.failure_reason ELSE NULL END,
            last_error = NEW.failure_reason,
            reconcile_locked_at = NEW.reconcile_locked_at,
            reconcile_locked_by = NEW.reconcile_locked_by,
            updated_at = now()
        WHERE id = OLD.id
        RETURNING * INTO binding_row;

        NEW.updated_at := binding_row.updated_at;
        RETURN NEW;
    ELSE
        SELECT binding.sender_asset_id
        INTO asset_id
        FROM sender_provider_bindings AS binding
        WHERE binding.id = OLD.id;

        DELETE FROM sender_provider_bindings WHERE id = OLD.id;
        DELETE FROM sender_assets AS asset
        WHERE asset.id = asset_id
          AND NOT EXISTS (
              SELECT 1
              FROM sender_provider_bindings AS binding
              WHERE binding.sender_asset_id = asset.id
          );
        RETURN OLD;
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_write_sender_domains_compatibility ON sender_domains;
CREATE TRIGGER trg_write_sender_domains_compatibility
INSTEAD OF INSERT OR UPDATE OR DELETE ON sender_domains
FOR EACH ROW
EXECUTE FUNCTION write_sender_domains_compatibility();
