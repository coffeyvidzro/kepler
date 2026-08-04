CREATE TABLE IF NOT EXISTS team_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_team_tokens_name_not_empty CHECK (length(trim(name)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_team_tokens_team_id
    ON team_tokens (team_id);

CREATE INDEX IF NOT EXISTS idx_team_tokens_team_id_revoked_at
    ON team_tokens (team_id, revoked_at);

CREATE INDEX IF NOT EXISTS idx_team_tokens_token_hash
    ON team_tokens (token_hash);

CREATE INDEX IF NOT EXISTS idx_team_tokens_expires_at
    ON team_tokens (expires_at);
