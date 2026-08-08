CREATE OR REPLACE FUNCTION enforce_email_sender_domain()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.sender_domain_id IS NULL THEN
        RAISE EXCEPTION 'customer sender domain is required'
            USING ERRCODE = '23514';
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM domains AS domain_record
        WHERE domain_record.id = NEW.sender_domain_id
          AND domain_record.team_id = NEW.team_id
          AND domain_record.provider = lower(trim(NEW.delivery_provider))
          AND domain_record.provider_region = lower(trim(NEW.provider_region))
          AND domain_record.status = 'verified'
          AND domain_record.sending_enabled
          AND domain_record.disabled_at IS NULL
          AND domain_record.health_status <> 'degraded'
    ) THEN
        RAISE EXCEPTION 'customer sender domain is not verified, enabled, and healthy'
            USING ERRCODE = '23514';
    END IF;

    PERFORM 1
    FROM email_tenants
    WHERE team_id = NEW.team_id
      AND provider = lower(trim(NEW.delivery_provider))
      AND region = lower(trim(NEW.provider_region))
      AND status = 'active';

    IF NOT FOUND THEN
        RAISE EXCEPTION 'active customer email tenant is required'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_enforce_email_sender_domain ON email_messages;

CREATE TRIGGER trg_enforce_email_sender_domain
BEFORE INSERT OR UPDATE OF team_id, sender_domain_id, delivery_provider, provider_region
ON email_messages
FOR EACH ROW
EXECUTE FUNCTION enforce_email_sender_domain();
