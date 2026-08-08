CREATE TABLE IF NOT EXISTS email_provider_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_message_id UUID REFERENCES email_messages(id) ON DELETE SET NULL,
    provider TEXT NOT NULL,
    transport TEXT NOT NULL,
    provider_notification_id TEXT NOT NULL,
    provider_message_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    normalized_payload JSONB NOT NULL,
    provider_payload JSONB NOT NULL,
    processed_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_attempt_at TIMESTAMPTZ,
    last_error TEXT,
    dead_lettered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_email_provider_events_notification
        UNIQUE (provider, transport, provider_notification_id),
    CONSTRAINT chk_email_provider_events_provider CHECK (length(trim(provider)) > 0),
    CONSTRAINT chk_email_provider_events_transport CHECK (length(trim(transport)) > 0),
    CONSTRAINT chk_email_provider_events_notification CHECK (length(trim(provider_notification_id)) > 0),
    CONSTRAINT chk_email_provider_events_message CHECK (length(trim(provider_message_id)) > 0),
    CONSTRAINT chk_email_provider_events_type CHECK (event_type IN (
        'send', 'delivery', 'delivery_delay', 'bounce', 'complaint', 'reject',
        'rendering_failure', 'open', 'click', 'subscription'
    )),
    CONSTRAINT chk_email_provider_events_normalized CHECK (jsonb_typeof(normalized_payload) = 'object'),
    CONSTRAINT chk_email_provider_events_provider_payload CHECK (jsonb_typeof(provider_payload) = 'object'),
    CONSTRAINT chk_email_provider_events_attempt_count CHECK (attempt_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_email_provider_events_message_occurred
    ON email_provider_events (email_message_id, occurred_at DESC)
    WHERE email_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_email_provider_events_provider_message
    ON email_provider_events (provider, provider_message_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_email_provider_events_reconcile_due
    ON email_provider_events (next_attempt_at, id)
    WHERE processed_at IS NULL
      AND dead_lettered_at IS NULL
      AND next_attempt_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_email_provider_events_dead_lettered
    ON email_provider_events (dead_lettered_at DESC)
    WHERE dead_lettered_at IS NOT NULL;
