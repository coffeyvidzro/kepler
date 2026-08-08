CREATE TABLE IF NOT EXISTS topics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name VARCHAR(50) NOT NULL,
    description VARCHAR(200),
    default_subscription TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'public',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_topics_id_team
        UNIQUE (id, team_id),

    CONSTRAINT chk_topics_name_not_empty
        CHECK (length(btrim(name)) > 0),

    CONSTRAINT chk_topics_description_not_empty
        CHECK (description IS NULL OR length(btrim(description)) > 0),

    CONSTRAINT chk_topics_default_subscription
        CHECK (default_subscription IN ('opt_in', 'opt_out')),

    CONSTRAINT chk_topics_visibility
        CHECK (visibility IN ('public', 'private'))
);

CREATE INDEX IF NOT EXISTS idx_topics_team_created
    ON topics (team_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_topics_team_name
    ON topics (team_id, name, id);

CREATE OR REPLACE FUNCTION prevent_topic_default_subscription_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.default_subscription <> OLD.default_subscription THEN
        RAISE EXCEPTION 'topic default subscription cannot be changed'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_prevent_topic_default_subscription_update ON topics;
CREATE TRIGGER trg_prevent_topic_default_subscription_update
BEFORE UPDATE OF default_subscription ON topics
FOR EACH ROW
EXECUTE FUNCTION prevent_topic_default_subscription_update();

CREATE TABLE IF NOT EXISTS contact_topic_subscriptions (
    team_id UUID NOT NULL,
    contact_id UUID NOT NULL,
    topic_id UUID NOT NULL,
    subscription TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (contact_id, topic_id),

    CONSTRAINT fk_contact_topic_subscriptions_contact_team
        FOREIGN KEY (contact_id, team_id)
        REFERENCES contacts (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_contact_topic_subscriptions_topic_team
        FOREIGN KEY (topic_id, team_id)
        REFERENCES topics (id, team_id)
        ON DELETE CASCADE,

    CONSTRAINT chk_contact_topic_subscriptions_value
        CHECK (subscription IN ('opt_in', 'opt_out'))
);

CREATE INDEX IF NOT EXISTS idx_contact_topic_subscriptions_team_topic_contact
    ON contact_topic_subscriptions (team_id, topic_id, contact_id);

CREATE INDEX IF NOT EXISTS idx_contact_topic_subscriptions_team_contact_topic
    ON contact_topic_subscriptions (team_id, contact_id, topic_id);
