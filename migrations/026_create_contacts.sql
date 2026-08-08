CREATE TABLE IF NOT EXISTS contacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    first_name TEXT,
    last_name TEXT,
    unsubscribed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_contacts_id_team
        UNIQUE (id, team_id),

    CONSTRAINT chk_contacts_email_not_empty
        CHECK (length(btrim(email)) > 0),

    CONSTRAINT chk_contacts_first_name_not_empty
        CHECK (first_name IS NULL OR length(btrim(first_name)) > 0),

    CONSTRAINT chk_contacts_last_name_not_empty
        CHECK (last_name IS NULL OR length(btrim(last_name)) > 0)
);

-- Resend treats a contact as unique by email within a team. Email lookup and
-- uniqueness are case-insensitive, while the original spelling is preserved.
CREATE UNIQUE INDEX IF NOT EXISTS uq_contacts_team_email
    ON contacts (team_id, lower(email));

CREATE INDEX IF NOT EXISTS idx_contacts_team_created
    ON contacts (team_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_contacts_team_subscribed
    ON contacts (team_id, created_at DESC, id DESC)
    WHERE unsubscribed = false;


CREATE TABLE IF NOT EXISTS contact_properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    key VARCHAR(50) NOT NULL,
    value_type TEXT NOT NULL,
    fallback_string TEXT,
    fallback_number NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_contact_properties_team_key
        UNIQUE (team_id, key),

    -- Required by the tenant-safe, type-safe foreign key from
    -- contact_property_values.
    CONSTRAINT uq_contact_properties_id_team_type
        UNIQUE (id, team_id, value_type),

    CONSTRAINT chk_contact_properties_key
        CHECK (
            char_length(key) BETWEEN 1 AND 50
            AND key ~ '^[A-Za-z0-9_]+$'
        ),

    CONSTRAINT chk_contact_properties_value_type
        CHECK (value_type IN ('string', 'number')),

    CONSTRAINT chk_contact_properties_fallback_type
        CHECK (
            (value_type = 'string' AND fallback_number IS NULL)
            OR
            (value_type = 'number' AND fallback_string IS NULL)
        )
);

CREATE INDEX IF NOT EXISTS idx_contact_properties_team_created
    ON contact_properties (team_id, created_at DESC, id DESC);


CREATE TABLE IF NOT EXISTS contact_property_values (
    team_id UUID NOT NULL,
    contact_id UUID NOT NULL,
    contact_property_id UUID NOT NULL,
    value_type TEXT NOT NULL,
    string_value TEXT,
    number_value NUMERIC,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (contact_id, contact_property_id),

    CONSTRAINT fk_contact_property_values_contact_team
        FOREIGN KEY (contact_id, team_id)
        REFERENCES contacts (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_contact_property_values_property_team_type
        FOREIGN KEY (contact_property_id, team_id, value_type)
        REFERENCES contact_properties (id, team_id, value_type)
        ON DELETE CASCADE,

    CONSTRAINT chk_contact_property_values_typed_value
        CHECK (
            (
                value_type = 'string'
                AND string_value IS NOT NULL
                AND number_value IS NULL
            )
            OR
            (
                value_type = 'number'
                AND string_value IS NULL
                AND number_value IS NOT NULL
            )
        )
);

CREATE INDEX IF NOT EXISTS idx_contact_property_values_team_contact
    ON contact_property_values (team_id, contact_id);

-- These indexes make future property-based segment filtering efficient.
CREATE INDEX IF NOT EXISTS idx_contact_property_values_string
    ON contact_property_values (
        team_id,
        contact_property_id,
        string_value,
        contact_id
    )
    WHERE value_type = 'string';

CREATE INDEX IF NOT EXISTS idx_contact_property_values_number
    ON contact_property_values (
        team_id,
        contact_property_id,
        number_value,
        contact_id
    )
    WHERE value_type = 'number';
