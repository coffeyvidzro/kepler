CREATE TABLE IF NOT EXISTS segments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_segments_id_team
        UNIQUE (id, team_id),

    CONSTRAINT chk_segments_name_not_empty
        CHECK (length(btrim(name)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_segments_team_created
    ON segments (team_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_segments_team_name
    ON segments (team_id, name, id);


CREATE TABLE IF NOT EXISTS contact_segments (
    team_id UUID NOT NULL,
    contact_id UUID NOT NULL,
    segment_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (contact_id, segment_id),

    CONSTRAINT fk_contact_segments_contact_team
        FOREIGN KEY (contact_id, team_id)
        REFERENCES contacts (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_contact_segments_segment_team
        FOREIGN KEY (segment_id, team_id)
        REFERENCES segments (id, team_id)
        ON DELETE CASCADE
);

-- Supports resolving all contacts in a segment for Broadcast recipient snapshots.
CREATE INDEX IF NOT EXISTS idx_contact_segments_team_segment_contact
    ON contact_segments (team_id, segment_id, contact_id);

-- Supports listing all segments assigned to a contact.
CREATE INDEX IF NOT EXISTS idx_contact_segments_team_contact_segment
    ON contact_segments (team_id, contact_id, segment_id);
