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
    CONSTRAINT chk_webhook_deliveries_replay_count CHECK (replay_count >= 0),
    CONSTRAINT chk_webhook_deliveries_replay_state CHECK (
        (replay_count = 0 AND last_replayed_at IS NULL)
        OR (replay_count > 0 AND last_replayed_at IS NOT NULL)
    ),
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

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_replayed
    ON webhook_deliveries (last_replayed_at DESC)
    WHERE replay_count > 0;

CREATE TABLE IF NOT EXISTS webhook_delivery_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL REFERENCES webhook_deliveries(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL,
    outcome TEXT NOT NULL,
    request_timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ NOT NULL,
    duration_ms BIGINT NOT NULL,
    response_status INTEGER,
    response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    response_body TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_webhook_delivery_attempts_number
        UNIQUE (delivery_id, attempt_number),
    CONSTRAINT chk_webhook_delivery_attempts_number CHECK (attempt_number > 0),
    CONSTRAINT chk_webhook_delivery_attempts_outcome CHECK (outcome IN (
        'succeeded',
        'retryable_failure',
        'permanent_failure',
        'timeout',
        'network_error'
    )),
    CONSTRAINT chk_webhook_delivery_attempts_duration CHECK (duration_ms >= 0),
    CONSTRAINT chk_webhook_delivery_attempts_response_status CHECK (
        response_status IS NULL OR response_status BETWEEN 100 AND 599
    ),
    CONSTRAINT chk_webhook_delivery_attempts_response_headers CHECK (
        jsonb_typeof(response_headers) = 'object'
    ),
    CONSTRAINT chk_webhook_delivery_attempts_timestamps CHECK (
        request_timestamp <= started_at
        AND completed_at >= started_at
    ),
    CONSTRAINT chk_webhook_delivery_attempts_outcome_fields CHECK (
        (
            outcome = 'succeeded'
            AND response_status BETWEEN 200 AND 299
            AND error_message IS NULL
        )
        OR (
            outcome IN ('retryable_failure', 'permanent_failure')
            AND (response_status IS NOT NULL OR error_message IS NOT NULL)
        )
        OR (
            outcome IN ('timeout', 'network_error')
            AND response_status IS NULL
            AND length(trim(error_message)) > 0
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_delivery_created
    ON webhook_delivery_attempts (delivery_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_webhook_delivery_attempts_outcome_created
    ON webhook_delivery_attempts (outcome, created_at DESC);
