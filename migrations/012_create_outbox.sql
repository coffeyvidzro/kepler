CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_outbox_subject CHECK (length(trim(subject)) > 0 AND subject !~ '[[:space:]]'),
    CONSTRAINT chk_outbox_aggregate_type CHECK (length(trim(aggregate_type)) > 0),
    CONSTRAINT chk_outbox_headers_object CHECK (jsonb_typeof(headers) = 'object'),
    CONSTRAINT chk_outbox_attempts_non_negative CHECK (attempts >= 0),
    CONSTRAINT chk_outbox_lock_pair CHECK (
        (locked_at IS NULL AND locked_by IS NULL)
        OR (locked_at IS NOT NULL AND locked_by IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_pending
    ON outbox_events (available_at, created_at)
    WHERE published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_outbox_events_locked
    ON outbox_events (locked_at)
    WHERE published_at IS NULL AND locked_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
    ON outbox_events (aggregate_type, aggregate_id, created_at DESC);

CREATE TABLE IF NOT EXISTS processed_events (
    consumer_name TEXT NOT NULL,
    event_id UUID NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,

    PRIMARY KEY (consumer_name, event_id),
    CONSTRAINT chk_processed_events_consumer CHECK (length(trim(consumer_name)) > 0),
    CONSTRAINT chk_processed_events_metadata_object CHECK (jsonb_typeof(metadata) = 'object')
);
