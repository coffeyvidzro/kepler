-- Create the channel-neutral provider-attempt ledger. Domains and Sender IDs
-- are created earlier so attempts can reference the correct channel-specific
-- sender identity.

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
    sender_domain_id UUID REFERENCES domains(id) ON DELETE SET NULL,
    sender_id UUID REFERENCES sender_ids(id) ON DELETE SET NULL,
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

    CONSTRAINT uq_message_delivery_attempts_email_reference
        UNIQUE (id, email_message_id, team_id),

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

    CONSTRAINT chk_message_delivery_attempts_sender_reference
        CHECK (
            (channel = 'email' AND sender_id IS NULL)
            OR (channel = 'sms' AND sender_domain_id IS NULL)
        ),

    CONSTRAINT chk_message_delivery_attempts_number
        CHECK (attempt_number > 0),

    CONSTRAINT chk_message_delivery_attempts_status
        CHECK (status IN (
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
        )),

    CONSTRAINT chk_message_delivery_attempts_provider
        CHECK (provider IS NULL OR length(trim(provider)) > 0),

    CONSTRAINT chk_message_delivery_attempts_provider_account
        CHECK (length(trim(provider_account)) > 0),

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
            AND (submitted_at IS NULL OR submitted_at >= claimed_at)
            AND (terminal_at IS NULL OR terminal_at >= claimed_at)
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

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_sender_domain
    ON message_delivery_attempts (sender_domain_id)
    WHERE sender_domain_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_sender_id
    ON message_delivery_attempts (sender_id)
    WHERE sender_id IS NOT NULL;

ALTER TABLE email_messages
    ADD CONSTRAINT fk_email_messages_current_delivery_attempt
    FOREIGN KEY (current_delivery_attempt_id, id, team_id)
    REFERENCES message_delivery_attempts (id, email_message_id, team_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX IF NOT EXISTS idx_email_messages_current_delivery_attempt
    ON email_messages (current_delivery_attempt_id)
    WHERE current_delivery_attempt_id IS NOT NULL;
