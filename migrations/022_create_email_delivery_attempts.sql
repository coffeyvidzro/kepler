CREATE TABLE IF NOT EXISTS email_delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_message_id UUID NOT NULL REFERENCES email_messages(id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'claimed',
    provider TEXT NOT NULL,
    provider_message_id TEXT,
    error_code TEXT,
    error_message TEXT,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_email_delivery_attempts_number
        UNIQUE (email_message_id, attempt_number),
    CONSTRAINT chk_email_delivery_attempts_number
        CHECK (attempt_number > 0),
    CONSTRAINT chk_email_delivery_attempts_status CHECK (status IN (
        'claimed', 'request_started', 'submitted', 'retryable_failure',
        'failed', 'submission_unknown'
    )),
    CONSTRAINT chk_email_delivery_attempts_provider
        CHECK (length(trim(provider)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_email_delivery_attempts_message_created
    ON email_delivery_attempts (email_message_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_email_delivery_attempts_unknown
    ON email_delivery_attempts (updated_at)
    WHERE status = 'submission_unknown';

ALTER TABLE email_messages
    ADD COLUMN IF NOT EXISTS current_delivery_attempt_id UUID
        REFERENCES email_delivery_attempts(id) ON DELETE SET NULL;

ALTER TABLE email_messages
    DROP CONSTRAINT IF EXISTS chk_email_status;

ALTER TABLE email_messages
    ADD CONSTRAINT chk_email_status CHECK (status IN (
        'queued', 'processing', 'submission_unknown', 'submitted',
        'delivered', 'delayed', 'bounced', 'complained', 'rejected',
        'failed', 'canceled'
    ));

CREATE INDEX IF NOT EXISTS idx_email_messages_submission_unknown
    ON email_messages (updated_at)
    WHERE status = 'submission_unknown';
