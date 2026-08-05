CREATE TABLE IF NOT EXISTS usage_allowances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    meter TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    included_quantity BIGINT NOT NULL,
    consumed_quantity BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_usage_allowances_team_meter_period
        UNIQUE (team_id, meter, period_start, period_end),
    CONSTRAINT uq_usage_allowances_id_team_meter
        UNIQUE (id, team_id, meter),
    CONSTRAINT chk_usage_allowances_meter
        CHECK (
            length(trim(meter)) > 0
            AND meter = lower(trim(meter))
            AND meter !~ '[[:space:]]'
        ),
    CONSTRAINT chk_usage_allowances_period
        CHECK (period_end > period_start),
    CONSTRAINT chk_usage_allowances_included_quantity
        CHECK (included_quantity >= 0),
    CONSTRAINT chk_usage_allowances_consumed_quantity
        CHECK (
            consumed_quantity >= 0
            AND consumed_quantity <= included_quantity
        ),
    CONSTRAINT ex_usage_allowances_no_overlap
        EXCLUDE USING gist (
            team_id WITH =,
            meter WITH =,
            tstzrange(period_start, period_end, '[)') WITH &&
        )
);

CREATE INDEX IF NOT EXISTS idx_usage_allowances_team_meter_period
    ON usage_allowances (
        team_id,
        meter,
        period_start DESC,
        period_end DESC
    );

CREATE TABLE IF NOT EXISTS usage_authorizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    product TEXT NOT NULL,
    meter TEXT NOT NULL,
    reference_id TEXT NOT NULL,

    usage_allowance_id UUID,
    sms_rate_id UUID,
    product_rate_id UUID,

    billing_market CHAR(2) NOT NULL,
    destination_country CHAR(2),
    provider TEXT,
    route_type TEXT,

    total_quantity BIGINT NOT NULL,
    allowance_quantity BIGINT NOT NULL DEFAULT 0,
    billable_quantity BIGINT NOT NULL DEFAULT 0,
    unit_cost_units BIGINT NOT NULL DEFAULT 0,
    amount_units BIGINT NOT NULL DEFAULT 0,

    currency CHAR(3) NOT NULL,
    tier TEXT NOT NULL,
    priced_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_usage_authorizations_team_meter_reference
        UNIQUE (team_id, meter, reference_id),
    CONSTRAINT uq_usage_authorizations_id_team
        UNIQUE (id, team_id),

    CONSTRAINT fk_usage_authorizations_allowance_same_team_meter
        FOREIGN KEY (usage_allowance_id, team_id, meter)
        REFERENCES usage_allowances (id, team_id, meter)
        ON DELETE RESTRICT,

    CONSTRAINT fk_usage_authorizations_wallet_market_currency
        FOREIGN KEY (team_id, billing_market, currency)
        REFERENCES team_wallets (team_id, billing_market, currency)
        ON DELETE RESTRICT,

    CONSTRAINT fk_usage_authorizations_sms_rate_audit
        FOREIGN KEY (
            sms_rate_id,
            billing_market,
            destination_country,
            provider,
            route_type,
            tier,
            currency,
            unit_cost_units
        )
        REFERENCES sms_rates (
            id,
            billing_market,
            destination_country,
            provider,
            route_type,
            tier,
            currency,
            cost_units
        )
        ON DELETE RESTRICT,

    CONSTRAINT fk_usage_authorizations_product_rate_audit
        FOREIGN KEY (
            product_rate_id,
            product,
            meter,
            billing_market,
            tier,
            currency,
            unit_cost_units
        )
        REFERENCES product_rates (
            id,
            product,
            meter,
            billing_market,
            tier,
            currency,
            cost_units
        )
        ON DELETE RESTRICT,

    CONSTRAINT chk_usage_authorizations_product
        CHECK (
            length(trim(product)) > 0
            AND product = lower(trim(product))
            AND product !~ '[[:space:]]'
        ),
    CONSTRAINT chk_usage_authorizations_meter
        CHECK (
            length(trim(meter)) > 0
            AND meter = lower(trim(meter))
            AND meter !~ '[[:space:]]'
        ),
    CONSTRAINT chk_usage_authorizations_reference
        CHECK (length(trim(reference_id)) > 0),
    CONSTRAINT chk_usage_authorizations_quantities
        CHECK (
            total_quantity > 0
            AND allowance_quantity >= 0
            AND billable_quantity >= 0
            AND allowance_quantity + billable_quantity = total_quantity
        ),
    CONSTRAINT chk_usage_authorizations_allowance
        CHECK (
            (allowance_quantity = 0 AND usage_allowance_id IS NULL)
            OR (allowance_quantity > 0 AND usage_allowance_id IS NOT NULL)
        ),
    CONSTRAINT chk_usage_authorizations_cost
        CHECK (
            unit_cost_units >= 0
            AND amount_units >= 0
            AND (
                (billable_quantity = 0 AND unit_cost_units = 0 AND amount_units = 0)
                OR (
                    billable_quantity > 0
                    AND unit_cost_units > 0
                    AND amount_units = billable_quantity * unit_cost_units
                )
            )
        ),
    CONSTRAINT chk_usage_authorizations_tier
        CHECK (tier IN ('growth', 'scale', 'enterprise')),
    CONSTRAINT chk_usage_authorizations_sms_context
        CHECK (
            (
                product = 'sms'
                AND meter = 'sms_segment'
                AND destination_country IS NOT NULL
                AND provider IS NOT NULL
                AND route_type IS NOT NULL
            )
            OR
            (
                product <> 'sms'
                AND destination_country IS NULL
                AND provider IS NULL
                AND route_type IS NULL
                AND sms_rate_id IS NULL
            )
        ),
    CONSTRAINT chk_usage_authorizations_rate_source
        CHECK (
            (
                billable_quantity = 0
                AND product_rate_id IS NULL
                AND sms_rate_id IS NULL
            )
            OR
            (
                billable_quantity > 0
                AND product = 'sms'
                AND sms_rate_id IS NOT NULL
                AND product_rate_id IS NULL
            )
            OR
            (
                billable_quantity > 0
                AND product <> 'sms'
                AND product_rate_id IS NOT NULL
                AND sms_rate_id IS NULL
            )
        )
);

ALTER TABLE wallet_ledger
    ADD CONSTRAINT fk_wallet_ledger_usage_authorization_same_team
    FOREIGN KEY (usage_authorization_id, team_id)
    REFERENCES usage_authorizations (id, team_id)
    ON DELETE RESTRICT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_wallet_ledger_usage_authorization
    ON wallet_ledger (usage_authorization_id)
    WHERE usage_authorization_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_team_created
    ON usage_authorizations (team_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_team_meter_created
    ON usage_authorizations (team_id, meter, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_usage_allowance
    ON usage_authorizations (usage_allowance_id, created_at DESC)
    WHERE usage_allowance_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_sms_rate
    ON usage_authorizations (sms_rate_id, created_at DESC)
    WHERE sms_rate_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_product_rate
    ON usage_authorizations (product_rate_id, created_at DESC)
    WHERE product_rate_id IS NOT NULL;
