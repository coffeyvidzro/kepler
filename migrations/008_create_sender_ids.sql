CREATE TABLE IF NOT EXISTS sender_ids (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name VARCHAR(11) NOT NULL,
    country_code TEXT NOT NULL,
    purpose TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',

    provider TEXT,
    provider_status TEXT,
    provider_whitelisted BOOLEAN NOT NULL DEFAULT false,
    provider_submitted_at TIMESTAMPTZ,
    provider_last_checked_at TIMESTAMPTZ,
    next_status_check_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    provider_attempts INTEGER NOT NULL DEFAULT 0,
    provider_error TEXT,
    registration_locked_at TIMESTAMPTZ,
    registration_locked_by TEXT,
    rejection_reason TEXT,

    approved_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    suspended_at TIMESTAMPTZ,

    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_sender_ids_country_code
        CHECK (country_code ~ '^[A-Z]{2}$'),

    CONSTRAINT chk_sender_ids_name_not_empty
        CHECK (length(trim(name)) > 0),

    CONSTRAINT chk_sender_ids_name_characters
        CHECK (name ~ '^[A-Za-z0-9]+$'),

    CONSTRAINT chk_sender_ids_name_has_letter
        CHECK (name ~ '[A-Za-z]'),

    CONSTRAINT chk_sender_ids_purpose_not_empty
        CHECK (length(trim(purpose)) > 0),

    CONSTRAINT chk_sender_ids_provider_attempts
        CHECK (provider_attempts >= 0),

    CONSTRAINT chk_sender_ids_status
        CHECK (
            status IN (
                'pending',
                'approved',
                'rejected',
                'suspended',
                'inactive'
            )
        )
);

-- The same tenant cannot register the same sender ID twice
-- for the same country.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sender_ids_team_country_sender
    ON sender_ids (
        team_id,
        country_code,
        lower(name)
    );

CREATE INDEX IF NOT EXISTS idx_sender_ids_team_id
    ON sender_ids (team_id);

CREATE INDEX IF NOT EXISTS idx_sender_ids_team_id_status
    ON sender_ids (team_id, status);

CREATE INDEX IF NOT EXISTS idx_sender_ids_country_code_status
    ON sender_ids (country_code, status);

CREATE INDEX IF NOT EXISTS idx_sender_ids_provider_reconciliation
    ON sender_ids (provider, next_status_check_at, created_at)
    WHERE status = 'pending';
