CREATE TABLE IF NOT EXISTS email_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    sender_domain_id UUID REFERENCES sender_domains(id) ON DELETE SET NULL,
    delivery_provider TEXT NOT NULL DEFAULT 'aws_ses',
    provider_region TEXT NOT NULL DEFAULT 'us-east-1',
    message_type TEXT NOT NULL DEFAULT 'transactional',
    from_email TEXT NOT NULL,
    from_name TEXT,
    reply_to_email TEXT,
    to_email TEXT NOT NULL,
    to_name TEXT,
    subject TEXT NOT NULL,
    html_body TEXT,
    text_body TEXT,
    status TEXT NOT NULL DEFAULT 'queued',
    provider TEXT,
    provider_message_id TEXT,
    error_code TEXT,
    error_message TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    recipients JSONB NOT NULL DEFAULT '{"to":[],"cc":[],"bcc":[],"reply_to":[]}'::jsonb,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    scheduled_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processing_at TIMESTAMPTZ,
    submitted_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_email_body_present CHECK (html_body IS NOT NULL OR text_body IS NOT NULL),
    CONSTRAINT chk_email_message_type CHECK (message_type IN ('transactional', 'marketing')),
    CONSTRAINT chk_email_status CHECK (status IN (
        'queued', 'processing', 'submitted', 'delivered', 'delayed',
        'bounced', 'complained', 'rejected', 'failed', 'canceled'
    )),
    CONSTRAINT chk_email_metadata_object CHECK (jsonb_typeof(metadata) = 'object'),
    CONSTRAINT chk_email_recipients_object CHECK (jsonb_typeof(recipients) = 'object'),
    CONSTRAINT chk_email_headers_object CHECK (jsonb_typeof(headers) = 'object'),
    CONSTRAINT chk_email_attachments_array CHECK (jsonb_typeof(attachments) = 'array'),
    CONSTRAINT chk_email_tags_array CHECK (jsonb_typeof(tags) = 'array'),
    CONSTRAINT chk_email_delivery_provider_present CHECK (length(trim(delivery_provider)) > 0),
    CONSTRAINT chk_email_provider_region_present CHECK (length(trim(provider_region)) > 0),
    CONSTRAINT chk_email_from_present CHECK (length(trim(from_email)) > 0),
    CONSTRAINT chk_email_to_present CHECK (length(trim(to_email)) > 0),
    CONSTRAINT chk_email_subject_present CHECK (length(trim(subject)) > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_messages_id_team
    ON email_messages (id, team_id);

CREATE INDEX IF NOT EXISTS idx_email_messages_team_created
    ON email_messages (team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_email_messages_sender_domain
    ON email_messages (sender_domain_id)
    WHERE sender_domain_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_messages_provider_message
    ON email_messages (provider, provider_message_id)
    WHERE provider IS NOT NULL AND provider_message_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_email_messages_team_scheduled
    ON email_messages (team_id, scheduled_at)
    WHERE status = 'queued' AND scheduled_at IS NOT NULL;
