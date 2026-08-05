CREATE OR REPLACE FUNCTION enforce_customer_email_tenant_route()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.sender_domain_id IS NULL THEN
        RAISE EXCEPTION 'customer sender binding is required'
            USING ERRCODE = '23514';
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
        WHERE binding.id = NEW.sender_domain_id
          AND asset.channel = 'email'
          AND binding.provider = CASE lower(trim(NEW.delivery_provider))
              WHEN 'aws_ses' THEN 'ses'
              ELSE lower(trim(NEW.delivery_provider))
          END
          AND binding.region = lower(trim(NEW.provider_region))
          AND binding.status = 'active'
          AND binding.verified
          AND binding.disabled_at IS NULL
          AND binding.health_status <> 'degraded'
    ) THEN
        RAISE EXCEPTION 'customer sender binding is not verified, enabled, and healthy'
            USING ERRCODE = '23514';
    END IF;

    -- Email tenant provider identifiers remain application-facing. Sender
    -- bindings use the canonical provider ID, so only the trust-plane lookup is
    -- normalized above.
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

DROP TRIGGER IF EXISTS trg_enforce_customer_email_tenant_route ON email_messages;

CREATE TRIGGER trg_enforce_customer_email_tenant_route
BEFORE INSERT ON email_messages
FOR EACH ROW
EXECUTE FUNCTION enforce_customer_email_tenant_route();
