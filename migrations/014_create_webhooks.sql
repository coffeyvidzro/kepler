CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    signing_secret BYTEA NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    subscribed_events TEXT[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    disabled_at TIMESTAMPTZ,
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    last_failure_at TIMESTAMPTZ,
    disabled_reason TEXT,

    CONSTRAINT chk_webhook_endpoint_url CHECK (
        length(trim(url)) > 0 AND url ~ '^https://'
    ),
    CONSTRAINT chk_webhook_endpoint_secret CHECK (octet_length(signing_secret) > 0),
    CONSTRAINT chk_webhook_endpoint_events CHECK (
        cardinality(subscribed_events) > 0
        AND array_position(subscribed_events, '') IS NULL
    ),
    CONSTRAINT chk_webhook_endpoint_disabled CHECK (
        (enabled AND disabled_at IS NULL)
        OR (NOT enabled AND disabled_at IS NOT NULL)
    ),
    CONSTRAINT chk_webhook_endpoint_consecutive_failures CHECK (
        consecutive_failures >= 0
    ),
    CONSTRAINT chk_webhook_endpoint_disabled_reason CHECK (
        disabled_reason IS NULL OR disabled_reason IN ('manual', 'failure_threshold')
    )
);

CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_team_created
    ON webhook_endpoints (team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_team_enabled
    ON webhook_endpoints (team_id, id)
    WHERE enabled;

CREATE TABLE IF NOT EXISTS webhook_events (
    id UUID PRIMARY KEY,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id UUID,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_webhook_event_type CHECK (
        length(trim(event_type)) > 0 AND event_type !~ '[[:space:]]'
    ),
    CONSTRAINT chk_webhook_event_object_type CHECK (length(trim(object_type)) > 0),
    CONSTRAINT chk_webhook_event_payload CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_webhook_events_team_created
    ON webhook_events (team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_events_object
    ON webhook_events (team_id, object_type, object_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL REFERENCES webhook_events(id) ON DELETE CASCADE,
    endpoint_id UUID NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    replay_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_attempt_at TIMESTAMPTZ,
    last_replayed_at TIMESTAMPTZ,
    response_status INTEGER,
    response_body TEXT,
    last_error TEXT,
    delivered_at TIMESTAMPTZ,
    locked_at TIMESTAMPTZ,
    locked_by TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_webhook_delivery_event_endpoint UNIQUE (event_id, endpoint_id),
    CONSTRAINT chk_webhook_delivery_status CHECK (
        status IN ('pending', 'retrying', 'succeeded', 'failed', 'canceled')
    ),
    CONSTRAINT chk_webhook_delivery_attempts CHECK (attempt_count >= 0),
    CONSTRAINT chk_webhook_delivery_response_status CHECK (
        response_status IS NULL OR response_status BETWEEN 100 AND 599
    ),
    CONSTRAINT chk_webhook_delivery_lock_pair CHECK (
        (locked_at IS NULL AND locked_by IS NULL)
        OR (locked_at IS NOT NULL AND locked_by IS NOT NULL)
    ),
    CONSTRAINT chk_webhook_delivery_succeeded CHECK (
        status <> 'succeeded' OR delivered_at IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending
    ON webhook_deliveries (next_attempt_at, created_at)
    WHERE status IN ('pending', 'retrying');

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_event_created
    ON webhook_deliveries (event_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint_created
    ON webhook_deliveries (endpoint_id, created_at DESC);
