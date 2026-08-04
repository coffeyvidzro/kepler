CREATE TABLE IF NOT EXISTS team_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    status TEXT NOT NULL DEFAULT 'pending',
    token_hash TEXT NOT NULL UNIQUE,
    invited_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    declined_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_team_invitations_role CHECK (role IN ('admin', 'member')),
    CONSTRAINT chk_team_invitations_status CHECK (status IN ('pending', 'accepted', 'declined', 'revoked'))
);

CREATE INDEX IF NOT EXISTS idx_team_invitations_team_status
    ON team_invitations (team_id, status);

CREATE INDEX IF NOT EXISTS idx_team_invitations_email_status
    ON team_invitations (email, status);

CREATE INDEX IF NOT EXISTS idx_team_invitations_expires_at
    ON team_invitations (expires_at);
