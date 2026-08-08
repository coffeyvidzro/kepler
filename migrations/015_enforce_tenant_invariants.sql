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

CREATE OR REPLACE FUNCTION enforce_sms_sender_id()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.sender_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM sender_ids AS sender_id
        WHERE sender_id.id = NEW.sender_id
          AND sender_id.team_id = NEW.team_id
          AND sender_id.status = 'approved'
          AND sender_id.provider_whitelisted
          AND sender_id.country_code = NEW.destination_country
    ) THEN
        RAISE EXCEPTION 'SMS sender ID is not approved for this team and destination'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_sms_sender_id ON sms_messages;
CREATE TRIGGER trg_enforce_sms_sender_id
BEFORE INSERT OR UPDATE OF team_id, sender_id, destination_country ON sms_messages
FOR EACH ROW
EXECUTE FUNCTION enforce_sms_sender_id();

CREATE OR REPLACE FUNCTION enforce_email_sender_domain()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.sender_domain_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM domains AS domain_record
        WHERE domain_record.id = NEW.sender_domain_id
          AND domain_record.team_id = NEW.team_id
          AND domain_record.provider = lower(trim(NEW.delivery_provider))
          AND domain_record.provider_region = lower(trim(NEW.provider_region))
    ) THEN
        RAISE EXCEPTION 'email sender domain does not belong to this team and route'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_email_sender_domain ON email_messages;
CREATE TRIGGER trg_enforce_email_sender_domain
BEFORE INSERT OR UPDATE OF team_id, sender_domain_id, delivery_provider, provider_region ON email_messages
FOR EACH ROW
EXECUTE FUNCTION enforce_email_sender_domain();

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
