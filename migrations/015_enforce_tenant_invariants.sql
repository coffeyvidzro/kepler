CREATE OR REPLACE FUNCTION protect_last_active_team_owner()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.role = 'owner'
       AND OLD.status = 'active'
       AND (
           TG_OP = 'DELETE'
           OR NEW.team_id <> OLD.team_id
           OR NEW.role <> 'owner'
           OR NEW.status <> 'active'
       )
       AND NOT EXISTS (
           SELECT 1
           FROM team_members
           WHERE team_id = OLD.team_id
             AND user_id <> OLD.user_id
             AND role = 'owner'
             AND status = 'active'
       ) THEN
        RAISE EXCEPTION 'team must retain an active owner'
            USING ERRCODE = '23514';
    END IF;
    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$$;

DROP TRIGGER IF EXISTS trg_protect_last_active_team_owner ON team_members;
CREATE TRIGGER trg_protect_last_active_team_owner
BEFORE DELETE OR UPDATE OF team_id, role, status ON team_members
FOR EACH ROW
EXECUTE FUNCTION protect_last_active_team_owner();

CREATE UNIQUE INDEX IF NOT EXISTS idx_sender_ids_id_team
    ON sender_ids (id, team_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sender_domains_id_team
    ON sender_domains (id, team_id);

ALTER TABLE sms_messages
    ADD CONSTRAINT fk_sms_sender_same_team
    FOREIGN KEY (sender_id, team_id)
    REFERENCES sender_ids (id, team_id);

ALTER TABLE email_messages
    ADD CONSTRAINT fk_email_sender_domain_same_team
    FOREIGN KEY (sender_domain_id, team_id)
    REFERENCES sender_domains (id, team_id);

CREATE OR REPLACE FUNCTION enforce_webhook_delivery_team()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM webhook_events AS event
        JOIN webhook_endpoints AS endpoint ON endpoint.id = NEW.endpoint_id
        WHERE event.id = NEW.event_id
          AND event.team_id = endpoint.team_id
    ) THEN
        RAISE EXCEPTION 'webhook event and endpoint must belong to the same team'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_webhook_delivery_team ON webhook_deliveries;
CREATE TRIGGER trg_enforce_webhook_delivery_team
BEFORE INSERT OR UPDATE OF event_id, endpoint_id ON webhook_deliveries
FOR EACH ROW
EXECUTE FUNCTION enforce_webhook_delivery_team();
