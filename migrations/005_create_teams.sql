CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    market_code VARCHAR(2) NOT NULL,
    phone TEXT NOT NULL,
    address TEXT NOT NULL,
    website TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_teams_status CHECK (status IN ('active', 'disabled'))
);

CREATE TABLE IF NOT EXISTS team_members (
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (team_id, user_id),
    CONSTRAINT chk_team_members_role CHECK (role IN ('owner', 'admin', 'member')),
    CONSTRAINT chk_team_members_status CHECK (status IN ('active', 'suspended', 'invited'))
);

CREATE INDEX IF NOT EXISTS idx_teams_status
    ON teams (status);

CREATE INDEX IF NOT EXISTS idx_teams_created_by
    ON teams (created_by);

CREATE INDEX IF NOT EXISTS idx_team_members_user_id
    ON team_members (user_id);

CREATE INDEX IF NOT EXISTS idx_team_members_team_id_status
    ON team_members (team_id, status);

CREATE INDEX IF NOT EXISTS idx_team_members_user_id_status
    ON team_members (user_id, status);
