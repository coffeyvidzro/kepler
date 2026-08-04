CREATE TABLE IF NOT EXISTS audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID,
    actor_type TEXT NOT NULL,
    actor_user_id UUID,
    actor_session_id TEXT,
    actor_token_id UUID,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    outcome TEXT NOT NULL DEFAULT 'success',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id TEXT,
    ip_address TEXT,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_audit_events_actor_type CHECK (
        actor_type IN ('user', 'team_token', 'system')
    ),
    CONSTRAINT chk_audit_events_outcome CHECK (
        outcome IN ('success', 'failure')
    ),
    CONSTRAINT chk_audit_events_action CHECK (length(trim(action)) > 0),
    CONSTRAINT chk_audit_events_resource_type CHECK (length(trim(resource_type)) > 0),
    CONSTRAINT chk_audit_events_metadata CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_audit_events_team_created
    ON audit_events (team_id, created_at DESC, id DESC)
    WHERE team_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_events_user_created
    ON audit_events (actor_user_id, created_at DESC)
    WHERE actor_user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_audit_events_action_created
    ON audit_events (action, created_at DESC);

-- Audit events are append-only by application convention. No UPDATE or DELETE
-- queries are generated for this table.
