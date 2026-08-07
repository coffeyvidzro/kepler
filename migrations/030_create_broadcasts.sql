CREATE TABLE IF NOT EXISTS broadcasts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    segment_id UUID NOT NULL,
    topic_id UUID,
    template_id UUID NOT NULL,
    template_version_id UUID,
    variable_bindings JSONB NOT NULL DEFAULT '{}'::jsonb,
    scheduled_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ,
    recipients_materialized_at TIMESTAMPTZ,
    audience_count BIGINT NOT NULL DEFAULT 0,
    eligible_count BIGINT NOT NULL DEFAULT 0,
    suppressed_count BIGINT NOT NULL DEFAULT 0,
    queued_count BIGINT NOT NULL DEFAULT 0,
    failed_count BIGINT NOT NULL DEFAULT 0,
    revision BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT uq_broadcasts_id_team UNIQUE (id, team_id),
    CONSTRAINT fk_broadcasts_segment_team FOREIGN KEY (segment_id, team_id)
        REFERENCES segments (id, team_id) ON DELETE RESTRICT,
    CONSTRAINT fk_broadcasts_topic_team FOREIGN KEY (topic_id, team_id)
        REFERENCES topics (id, team_id) ON DELETE RESTRICT,
    CONSTRAINT fk_broadcasts_template_team FOREIGN KEY (template_id, team_id)
        REFERENCES message_templates (id, team_id) ON DELETE RESTRICT,
    CONSTRAINT fk_broadcasts_template_version FOREIGN KEY (template_version_id)
        REFERENCES message_template_versions (id) ON DELETE RESTRICT,
    CONSTRAINT chk_broadcasts_name_not_empty CHECK (length(btrim(name)) > 0),
    CONSTRAINT chk_broadcasts_status CHECK (status IN ('draft','scheduled','queued','sent','failed','canceled')),
    CONSTRAINT chk_broadcasts_revision CHECK (revision > 0)
);

CREATE INDEX IF NOT EXISTS idx_broadcasts_team_created
    ON broadcasts (team_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_broadcasts_due
    ON broadcasts (scheduled_at, id)
    WHERE status = 'scheduled' AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_broadcasts_pending_materialization
    ON broadcasts (queued_at, id)
    WHERE status = 'queued'
      AND recipients_materialized_at IS NULL
      AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS broadcast_recipients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    broadcast_id UUID NOT NULL,
    contact_id UUID,
    email TEXT NOT NULL,
    normalized_email TEXT NOT NULL,
    first_name TEXT,
    last_name TEXT,
    contact_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending',
    exclusion_reason TEXT,
    email_message_id UUID,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_error_code TEXT,
    last_error_message TEXT,
    failed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    queued_at TIMESTAMPTZ,

    CONSTRAINT fk_broadcast_recipients_broadcast_team FOREIGN KEY (broadcast_id, team_id)
        REFERENCES broadcasts (id, team_id) ON DELETE CASCADE,
    CONSTRAINT fk_broadcast_recipients_email_message FOREIGN KEY (email_message_id)
        REFERENCES email_messages (id) ON DELETE SET NULL,
    CONSTRAINT uq_broadcast_recipients_contact UNIQUE (broadcast_id, contact_id),
    CONSTRAINT uq_broadcast_recipients_email UNIQUE (broadcast_id, normalized_email),
    CONSTRAINT chk_broadcast_recipients_status CHECK (status IN ('pending','excluded','queued','failed')),
    CONSTRAINT chk_broadcast_recipients_email_not_empty CHECK (length(btrim(email)) > 0),
    CONSTRAINT chk_broadcast_recipients_attempt_count CHECK (attempt_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_broadcast_recipients_broadcast_status
    ON broadcast_recipients (broadcast_id, status, id);

CREATE INDEX IF NOT EXISTS idx_broadcast_recipients_pending_fanout
    ON broadcast_recipients (next_attempt_at, broadcast_id, id)
    WHERE status = 'pending';
