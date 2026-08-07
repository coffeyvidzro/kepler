CREATE TABLE IF NOT EXISTS message_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    alias VARCHAR(100),
    current_version_id UUID,
    published_version_id UUID,
    next_version_number INTEGER NOT NULL DEFAULT 1,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT uq_message_templates_id_team UNIQUE (id, team_id),
    CONSTRAINT chk_message_templates_name CHECK (length(btrim(name)) > 0),
    CONSTRAINT chk_message_templates_alias CHECK (alias ~ '^[A-Za-z0-9_-]+$'),
    CONSTRAINT chk_message_templates_next_version CHECK (next_version_number > 0),
    CONSTRAINT chk_message_templates_publication CHECK (
        (published_version_id IS NULL AND published_at IS NULL)
        OR (published_version_id IS NOT NULL AND published_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_message_templates_team_alias
    ON message_templates (team_id, lower(alias))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_message_templates_team_created
    ON message_templates (team_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS message_template_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    template_id UUID NOT NULL,
    version_number INTEGER NOT NULL,
    from_email TEXT,
    from_name TEXT,
    reply_to_email TEXT,
    subject TEXT NOT NULL,
    html_body TEXT NOT NULL,
    text_body TEXT,
    variables JSONB NOT NULL DEFAULT '[]'::jsonb,
    based_on_version_id UUID,
    change_note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_message_template_versions_number UNIQUE (template_id, version_number),
    CONSTRAINT uq_message_template_versions_id_template_team UNIQUE (id, template_id, team_id),
    CONSTRAINT fk_message_template_versions_template_team
        FOREIGN KEY (template_id, team_id)
        REFERENCES message_templates (id, team_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_message_template_versions_based_on
        FOREIGN KEY (based_on_version_id)
        REFERENCES message_template_versions (id)
        ON DELETE SET NULL,
    CONSTRAINT chk_message_template_versions_number CHECK (version_number > 0),
    CONSTRAINT chk_message_template_versions_html CHECK (length(btrim(html_body)) > 0),
    CONSTRAINT chk_message_template_versions_variables CHECK (jsonb_typeof(variables) = 'array')
);

ALTER TABLE message_templates
    ADD CONSTRAINT fk_message_templates_current_version
        FOREIGN KEY (current_version_id, id, team_id)
        REFERENCES message_template_versions (id, template_id, team_id)
        DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE message_templates
    ADD CONSTRAINT fk_message_templates_published_version
        FOREIGN KEY (published_version_id, id, team_id)
        REFERENCES message_template_versions (id, template_id, team_id)
        DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX IF NOT EXISTS idx_message_template_versions_template_created
    ON message_template_versions (team_id, template_id, version_number DESC);

CREATE TABLE IF NOT EXISTS message_template_publications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    template_id UUID NOT NULL,
    version_id UUID NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_message_template_publications_template_team
        FOREIGN KEY (template_id, team_id)
        REFERENCES message_templates (id, team_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_message_template_publications_version_template_team
        FOREIGN KEY (version_id, template_id, team_id)
        REFERENCES message_template_versions (id, template_id, team_id)
        ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_message_template_publications_template
    ON message_template_publications (team_id, template_id, published_at DESC);
