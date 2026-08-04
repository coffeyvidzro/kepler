CREATE TABLE IF NOT EXISTS idempotency_keys (
    scope TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'processing',
    response_status INTEGER,
    response_body BYTEA,
    response_content_type TEXT,
    response_headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    locked_until TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT idempotency_keys_pkey
        PRIMARY KEY (scope, idempotency_key),

    CONSTRAINT idempotency_keys_status_check
        CHECK (status IN ('processing', 'completed'))
);

CREATE INDEX IF NOT EXISTS idx_idempotency_keys_expires_at
    ON idempotency_keys (expires_at);
