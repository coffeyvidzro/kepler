ALTER TABLE email_messages
    DROP CONSTRAINT IF EXISTS chk_email_status;

ALTER TABLE email_messages
    ADD CONSTRAINT chk_email_status CHECK (status IN (
        'queued', 'processing', 'submission_unknown', 'submitted',
        'delivered', 'partially_delivered', 'delayed',
        'bounced', 'complained', 'rejected', 'failed',
        'partially_failed', 'canceled'
    ));

CREATE TABLE IF NOT EXISTS email_recipients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email_message_id UUID NOT NULL REFERENCES email_messages(id) ON DELETE CASCADE,
    recipient_email TEXT NOT NULL,
    recipient_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    last_event_type TEXT,
    last_event_at TIMESTAMPTZ,
    last_action TEXT,
    last_status_code TEXT,
    last_diagnostic_code TEXT,
    delivered_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_email_recipients_message_email
        UNIQUE (email_message_id, recipient_email),
    CONSTRAINT chk_email_recipients_email
        CHECK (length(trim(recipient_email)) > 0 AND recipient_email = lower(recipient_email)),
    CONSTRAINT chk_email_recipients_type
        CHECK (recipient_type IN ('to', 'cc', 'bcc', 'unknown')),
    CONSTRAINT chk_email_recipients_status
        CHECK (status IN (
            'pending', 'submitted', 'delayed', 'delivered',
            'bounced', 'complained', 'rejected', 'failed'
        )),
    CONSTRAINT chk_email_recipients_event_type
        CHECK (last_event_type IS NULL OR last_event_type IN (
            'send', 'delivery', 'delivery_delay', 'bounce',
            'complaint', 'reject', 'rendering_failure'
        ))
);

WITH recipient_addresses AS (
    SELECT
        message.id AS email_message_id,
        lower(trim(recipient.address ->> 'email')) AS recipient_email,
        recipient.recipient_type,
        recipient.priority,
        message.status AS message_status
    FROM email_messages AS message
    CROSS JOIN LATERAL (
        SELECT value AS address, 'to'::text AS recipient_type, 1 AS priority
        FROM jsonb_array_elements(COALESCE(message.recipients -> 'to', '[]'::jsonb))
        UNION ALL
        SELECT value AS address, 'cc'::text AS recipient_type, 2 AS priority
        FROM jsonb_array_elements(COALESCE(message.recipients -> 'cc', '[]'::jsonb))
        UNION ALL
        SELECT value AS address, 'bcc'::text AS recipient_type, 3 AS priority
        FROM jsonb_array_elements(COALESCE(message.recipients -> 'bcc', '[]'::jsonb))
    ) AS recipient
    WHERE length(trim(recipient.address ->> 'email')) > 0
), deduplicated AS (
    SELECT DISTINCT ON (email_message_id, recipient_email)
        email_message_id,
        recipient_email,
        recipient_type,
        message_status
    FROM recipient_addresses
    ORDER BY email_message_id, recipient_email, priority
)
INSERT INTO email_recipients (
    email_message_id,
    recipient_email,
    recipient_type,
    status
)
SELECT
    email_message_id,
    recipient_email,
    recipient_type,
    CASE message_status
        WHEN 'submitted' THEN 'submitted'
        WHEN 'delayed' THEN 'delayed'
        WHEN 'delivered' THEN 'delivered'
        WHEN 'partially_delivered' THEN 'delivered'
        WHEN 'bounced' THEN 'bounced'
        WHEN 'complained' THEN 'complained'
        WHEN 'rejected' THEN 'rejected'
        WHEN 'failed' THEN 'failed'
        WHEN 'partially_failed' THEN 'failed'
        ELSE 'pending'
    END
FROM deduplicated
ON CONFLICT (email_message_id, recipient_email) DO NOTHING;

CREATE OR REPLACE FUNCTION create_email_recipient_states()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO email_recipients (
        email_message_id,
        recipient_email,
        recipient_type,
        status
    )
    SELECT
        NEW.id,
        address.recipient_email,
        address.recipient_type,
        'pending'
    FROM (
        SELECT DISTINCT ON (recipient_email)
            recipient_email,
            recipient_type,
            priority
        FROM (
            SELECT lower(trim(value ->> 'email')) AS recipient_email, 'to'::text AS recipient_type, 1 AS priority
            FROM jsonb_array_elements(COALESCE(NEW.recipients -> 'to', '[]'::jsonb))
            UNION ALL
            SELECT lower(trim(value ->> 'email')) AS recipient_email, 'cc'::text AS recipient_type, 2 AS priority
            FROM jsonb_array_elements(COALESCE(NEW.recipients -> 'cc', '[]'::jsonb))
            UNION ALL
            SELECT lower(trim(value ->> 'email')) AS recipient_email, 'bcc'::text AS recipient_type, 3 AS priority
            FROM jsonb_array_elements(COALESCE(NEW.recipients -> 'bcc', '[]'::jsonb))
        ) AS candidates
        WHERE length(recipient_email) > 0
        ORDER BY recipient_email, priority
    ) AS address
    ON CONFLICT (email_message_id, recipient_email) DO NOTHING;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_create_email_recipient_states ON email_messages;

CREATE TRIGGER trg_create_email_recipient_states
AFTER INSERT ON email_messages
FOR EACH ROW
EXECUTE FUNCTION create_email_recipient_states();

CREATE INDEX IF NOT EXISTS idx_email_recipients_message_status
    ON email_recipients (email_message_id, status);

CREATE INDEX IF NOT EXISTS idx_email_recipients_email_updated
    ON email_recipients (recipient_email, updated_at DESC);
