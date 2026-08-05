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

CREATE OR REPLACE FUNCTION enforce_sms_sender_binding()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.sender_provider_binding_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM sender_provider_bindings AS binding
        JOIN sender_assets AS asset
          ON asset.id = binding.sender_asset_id
        JOIN sender_asset_grants AS grant_record
          ON grant_record.sender_asset_id = asset.id
         AND grant_record.team_id = NEW.team_id
         AND grant_record.channel = 'sms'
         AND grant_record.status = 'active'
        WHERE binding.id = NEW.sender_provider_binding_id
          AND asset.channel = 'sms'
          AND binding.status = 'active'
          AND binding.verified
          AND (
              binding.country_code IS NULL
              OR binding.country_code = NEW.destination_country
          )
    ) THEN
        RAISE EXCEPTION 'SMS sender binding is not active for this team and destination'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_sms_sender_binding ON sms_messages;
CREATE TRIGGER trg_enforce_sms_sender_binding
BEFORE INSERT OR UPDATE OF team_id, sender_provider_binding_id, destination_country ON sms_messages
FOR EACH ROW
EXECUTE FUNCTION enforce_sms_sender_binding();

CREATE OR REPLACE FUNCTION enforce_email_sender_binding()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.sender_provider_binding_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM sender_provider_bindings AS binding
        JOIN sender_assets AS asset
          ON asset.id = binding.sender_asset_id
        JOIN sender_asset_grants AS grant_record
          ON grant_record.sender_asset_id = asset.id
         AND grant_record.team_id = NEW.team_id
         AND grant_record.channel = 'email'
         AND grant_record.status = 'active'
        WHERE binding.id = NEW.sender_provider_binding_id
          AND asset.channel = 'email'
          AND binding.provider = CASE lower(trim(NEW.delivery_provider))
              WHEN 'aws_ses' THEN 'ses'
              ELSE lower(trim(NEW.delivery_provider))
          END
          AND binding.region = lower(trim(NEW.provider_region))
    ) THEN
        RAISE EXCEPTION 'email sender binding does not belong to this team and route'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_email_sender_binding ON email_messages;
CREATE TRIGGER trg_enforce_email_sender_binding
BEFORE INSERT OR UPDATE OF team_id, sender_provider_binding_id, delivery_provider, provider_region ON email_messages
FOR EACH ROW
EXECUTE FUNCTION enforce_email_sender_binding();

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
