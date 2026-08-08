CREATE TABLE IF NOT EXISTS suppressions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    origin TEXT NOT NULL DEFAULT 'manual',
    source_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_suppressions_id_team
        UNIQUE (id, team_id),

    CONSTRAINT chk_suppressions_email_not_empty
        CHECK (length(btrim(email)) > 0),

    CONSTRAINT chk_suppressions_origin
        CHECK (origin IN ('manual', 'bounce', 'complaint')),

    CONSTRAINT chk_suppressions_source_id_not_empty
        CHECK (source_id IS NULL OR length(btrim(source_id)) > 0)
);

-- A destination may be suppressed only once per team, regardless of email case.
CREATE UNIQUE INDEX IF NOT EXISTS uq_suppressions_team_email
    ON suppressions (team_id, lower(email));

CREATE INDEX IF NOT EXISTS idx_suppressions_team_created
    ON suppressions (team_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_suppressions_team_origin_created
    ON suppressions (team_id, origin, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_suppressions_team_source
    ON suppressions (team_id, source_id)
    WHERE source_id IS NOT NULL;
