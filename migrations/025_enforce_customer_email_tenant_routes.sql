CREATE OR REPLACE FUNCTION enforce_customer_email_tenant_route()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    -- The shared onboarding identity uses Dugble's platform sandbox tenant and
    -- does not require a dedicated email_tenants row for the customer team.
    IF NEW.sender_domain_id IS NULL THEN
        IF NEW.message_type <> 'transactional' THEN
            RAISE EXCEPTION 'onboarding identity supports transactional email only'
                USING ERRCODE = '23514';
        END IF;

        RETURN NEW;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM sender_domains AS domain
        WHERE domain.id = NEW.sender_domain_id
          AND domain.team_id = NEW.team_id
          AND domain.provider = NEW.delivery_provider
          AND domain.provider_region = NEW.provider_region
          AND domain.status = 'verified'
          AND domain.disabled_at IS NULL
          AND domain.health_status <> 'degraded'
    ) THEN
        RAISE EXCEPTION 'customer sender domain is not verified, enabled, and healthy'
            USING ERRCODE = '23514';
    END IF;

    -- Routing names and header keys are application-owned constants. The
    -- database enforces only relational tenant isolation and lifecycle state.
    PERFORM 1
    FROM email_tenants
    WHERE team_id = NEW.team_id
      AND provider = NEW.delivery_provider
      AND region = NEW.provider_region
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
