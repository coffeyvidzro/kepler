CREATE TABLE IF NOT EXISTS verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    channel TEXT NOT NULL DEFAULT 'sms',
    recipient TEXT NOT NULL,
    recipient_normalized TEXT NOT NULL,
    code_length INTEGER NOT NULL DEFAULT 6,
    ttl_seconds INTEGER NOT NULL DEFAULT 300,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    resend_cooldown_seconds INTEGER NOT NULL DEFAULT 30,
    max_resends INTEGER NOT NULL DEFAULT 3,
    status TEXT NOT NULL DEFAULT 'pending',
    locale TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    resend_count INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    approved_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_verifications_id_team UNIQUE (id, team_id),
    CONSTRAINT chk_verifications_channel CHECK (channel IN ('email', 'sms')),
    CONSTRAINT chk_verifications_recipient CHECK (length(trim(recipient)) > 0),
    CONSTRAINT chk_verifications_recipient_normalized CHECK (
        length(trim(recipient_normalized)) > 0
    ),
    CONSTRAINT chk_verifications_code_length CHECK (
        code_length BETWEEN 4 AND 10
    ),
    CONSTRAINT chk_verifications_ttl CHECK (
        ttl_seconds BETWEEN 30 AND 3600
    ),
    CONSTRAINT chk_verifications_max_attempts CHECK (
        max_attempts BETWEEN 1 AND 20
    ),
    CONSTRAINT chk_verifications_resend_cooldown CHECK (
        resend_cooldown_seconds BETWEEN 0 AND 3600
    ),
    CONSTRAINT chk_verifications_max_resends CHECK (
        max_resends BETWEEN 0 AND 20
    ),
    CONSTRAINT chk_verifications_status CHECK (status IN (
        'pending',
        'approved',
        'expired',
        'canceled',
        'max_attempts_reached',
        'delivery_failed'
    )),
    CONSTRAINT chk_verifications_metadata CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT chk_verifications_attempt_count CHECK (attempt_count >= 0),
    CONSTRAINT chk_verifications_resend_count CHECK (resend_count >= 0),
    CONSTRAINT chk_verifications_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_verifications_terminal_timestamps CHECK (
        (approved_at IS NULL OR status = 'approved')
        AND (expired_at IS NULL OR status = 'expired')
        AND (canceled_at IS NULL OR status = 'canceled')
        AND (failed_at IS NULL OR status IN ('max_attempts_reached', 'delivery_failed'))
        AND (status <> 'approved' OR approved_at IS NOT NULL)
        AND (status <> 'expired' OR expired_at IS NOT NULL)
        AND (status <> 'canceled' OR canceled_at IS NOT NULL)
        AND (
            status NOT IN ('max_attempts_reached', 'delivery_failed')
            OR failed_at IS NOT NULL
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_verifications_team_created
    ON verifications (team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_verifications_team_recipient
    ON verifications (team_id, recipient_normalized, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_verifications_pending_expiry
    ON verifications (expires_at, created_at)
    WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS verification_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    verification_id UUID NOT NULL,
    sequence INTEGER NOT NULL,
    code_hmac BYTEA NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    channel TEXT NOT NULL,
    email_message_id UUID,
    sms_message_id UUID,
    expires_at TIMESTAMPTZ NOT NULL,
    superseded_at TIMESTAMPTZ,
    dispatched_at TIMESTAMPTZ,
    delivery_failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_verification_challenges_sequence UNIQUE (verification_id, sequence),
    CONSTRAINT uq_verification_challenges_identity UNIQUE (id, verification_id, team_id),
    CONSTRAINT fk_verification_challenges_verification_same_team
        FOREIGN KEY (verification_id, team_id)
        REFERENCES verifications (id, team_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_verification_challenges_email_same_team
        FOREIGN KEY (email_message_id, team_id)
        REFERENCES email_messages (id, team_id)
        ON DELETE SET NULL (email_message_id),
    CONSTRAINT fk_verification_challenges_sms_same_team
        FOREIGN KEY (sms_message_id, team_id)
        REFERENCES sms_messages (id, team_id)
        ON DELETE SET NULL (sms_message_id),
    CONSTRAINT chk_verification_challenges_sequence CHECK (sequence > 0),
    CONSTRAINT chk_verification_challenges_hmac CHECK (octet_length(code_hmac) > 0),
    CONSTRAINT chk_verification_challenges_status CHECK (status IN (
        'queued',
        'dispatching',
        'dispatched',
        'delivery_failed',
        'superseded',
        'expired'
    )),
    CONSTRAINT chk_verification_challenges_channel CHECK (channel IN ('email', 'sms')),
    CONSTRAINT chk_verification_challenges_message_reference CHECK (
        num_nonnulls(email_message_id, sms_message_id) <= 1
    ),
    CONSTRAINT chk_verification_challenges_expiry CHECK (expires_at > created_at),
    CONSTRAINT chk_verification_challenges_state_timestamps CHECK (
        (status <> 'dispatched' OR dispatched_at IS NOT NULL)
        AND (status <> 'delivery_failed' OR delivery_failed_at IS NOT NULL)
        AND (status <> 'superseded' OR superseded_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_verification_challenges_verification_created
    ON verification_challenges (verification_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_verification_challenges_pending
    ON verification_challenges (expires_at, created_at)
    WHERE status IN ('queued', 'dispatching', 'dispatched');

CREATE INDEX IF NOT EXISTS idx_verification_challenges_email_message
    ON verification_challenges (email_message_id)
    WHERE email_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_verification_challenges_sms_message
    ON verification_challenges (sms_message_id)
    WHERE sms_message_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS verification_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    verification_id UUID NOT NULL,
    challenge_id UUID NOT NULL,
    result TEXT NOT NULL,
    ip_address_hash BYTEA,
    user_agent TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_verification_attempts_verification_same_team
        FOREIGN KEY (verification_id, team_id)
        REFERENCES verifications (id, team_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_verification_attempts_challenge
        FOREIGN KEY (challenge_id, verification_id, team_id)
        REFERENCES verification_challenges (id, verification_id, team_id)
        ON DELETE CASCADE,
    CONSTRAINT chk_verification_attempts_result CHECK (result IN (
        'approved',
        'incorrect',
        'expired',
        'superseded',
        'max_attempts_reached'
    )),
    CONSTRAINT chk_verification_attempts_ip_hash CHECK (
        ip_address_hash IS NULL OR octet_length(ip_address_hash) > 0
    ),
    CONSTRAINT chk_verification_attempts_metadata CHECK (
        jsonb_typeof(metadata) = 'object'
    )
);

CREATE INDEX IF NOT EXISTS idx_verification_attempts_verification_attempted
    ON verification_attempts (verification_id, attempted_at DESC);

CREATE INDEX IF NOT EXISTS idx_verification_attempts_team_attempted
    ON verification_attempts (team_id, attempted_at DESC);
