CREATE TABLE IF NOT EXISTS allowance_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product TEXT NOT NULL,
    meter TEXT NOT NULL,
    billing_market CHAR(2) NOT NULL
        REFERENCES billing_markets(code)
        ON DELETE RESTRICT,
    tier TEXT NOT NULL,
    included_quantity BIGINT NOT NULL,
    cadence TEXT NOT NULL DEFAULT 'monthly',
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_allowance_policies_grant_context
        UNIQUE (
            id,
            product,
            meter,
            billing_market,
            tier,
            included_quantity
        ),
    CONSTRAINT chk_allowance_policies_product
        CHECK (
            length(trim(product)) > 0
            AND product = lower(trim(product))
            AND product !~ '[[:space:]]'
        ),
    CONSTRAINT chk_allowance_policies_meter
        CHECK (
            length(trim(meter)) > 0
            AND meter = lower(trim(meter))
            AND meter !~ '[[:space:]]'
        ),
    CONSTRAINT chk_allowance_policies_tier
        CHECK (tier IN ('growth', 'scale', 'enterprise')),
    CONSTRAINT chk_allowance_policies_quantity
        CHECK (included_quantity > 0),
    CONSTRAINT chk_allowance_policies_cadence
        CHECK (cadence = 'monthly'),
    CONSTRAINT chk_allowance_policies_period
        CHECK (
            effective_from = (
                date_trunc(
                    'month',
                    effective_from AT TIME ZONE 'UTC'
                ) AT TIME ZONE 'UTC'
            )
            AND (
                effective_until IS NULL
                OR (
                    effective_until > effective_from
                    AND effective_until = (
                        date_trunc(
                            'month',
                            effective_until AT TIME ZONE 'UTC'
                        ) AT TIME ZONE 'UTC'
                    )
                )
            )
        ),
    CONSTRAINT ex_allowance_policies_no_overlap
        EXCLUDE USING gist (
            product WITH =,
            meter WITH =,
            billing_market WITH =,
            tier WITH =,
            cadence WITH =,
            tstzrange(
                effective_from,
                COALESCE(effective_until, 'infinity'::timestamptz),
                '[)'
            ) WITH &&
        )
);

CREATE INDEX IF NOT EXISTS idx_allowance_policies_lookup
    ON allowance_policies (
        product,
        meter,
        billing_market,
        tier,
        effective_from DESC
    );

CREATE TABLE IF NOT EXISTS usage_allowances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL,
    allowance_policy_id UUID NOT NULL,
    product TEXT NOT NULL,
    meter TEXT NOT NULL,
    billing_market CHAR(2) NOT NULL,
    tier TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    included_quantity BIGINT NOT NULL,
    consumed_quantity BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_usage_allowances_team_product_meter_period
        UNIQUE (
            team_id,
            product,
            meter,
            period_start,
            period_end
        ),
    CONSTRAINT uq_usage_allowances_authorization_context
        UNIQUE (
            id,
            team_id,
            product,
            meter,
            billing_market,
            tier
        ),
    CONSTRAINT fk_usage_allowances_policy_context
        FOREIGN KEY (
            allowance_policy_id,
            product,
            meter,
            billing_market,
            tier,
            included_quantity
        )
        REFERENCES allowance_policies (
            id,
            product,
            meter,
            billing_market,
            tier,
            included_quantity
        )
        ON DELETE RESTRICT,
    CONSTRAINT fk_usage_allowances_team_market
        FOREIGN KEY (team_id, billing_market)
        REFERENCES teams(id, market_code)
        ON DELETE RESTRICT,
    CONSTRAINT chk_usage_allowances_product
        CHECK (
            length(trim(product)) > 0
            AND product = lower(trim(product))
            AND product !~ '[[:space:]]'
        ),
    CONSTRAINT chk_usage_allowances_meter
        CHECK (
            length(trim(meter)) > 0
            AND meter = lower(trim(meter))
            AND meter !~ '[[:space:]]'
        ),
    CONSTRAINT chk_usage_allowances_tier
        CHECK (tier IN ('growth', 'scale', 'enterprise')),
    CONSTRAINT chk_usage_allowances_period
        CHECK (
            period_start = (
                date_trunc(
                    'month',
                    period_start AT TIME ZONE 'UTC'
                ) AT TIME ZONE 'UTC'
            )
            AND period_end = (
                date_trunc(
                    'month',
                    period_start AT TIME ZONE 'UTC'
                ) + interval '1 month'
            ) AT TIME ZONE 'UTC'
        ),
    CONSTRAINT chk_usage_allowances_included_quantity
        CHECK (included_quantity > 0),
    CONSTRAINT chk_usage_allowances_consumed_quantity
        CHECK (
            consumed_quantity >= 0
            AND consumed_quantity <= included_quantity
        ),
    CONSTRAINT ex_usage_allowances_no_overlap
        EXCLUDE USING gist (
            team_id WITH =,
            product WITH =,
            meter WITH =,
            tstzrange(period_start, period_end, '[)') WITH &&
        )
);

CREATE INDEX IF NOT EXISTS idx_usage_allowances_team_product_meter_period
    ON usage_allowances (
        team_id,
        product,
        meter,
        period_start DESC,
        period_end DESC
    );

CREATE INDEX IF NOT EXISTS idx_usage_allowances_policy
    ON usage_allowances (
        allowance_policy_id,
        period_start DESC
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

    CONSTRAINT uq_usage_authorizations_team_product_meter_reference
        UNIQUE (team_id, product, meter, reference_id),
    CONSTRAINT uq_usage_authorizations_id_team
        UNIQUE (id, team_id),

    CONSTRAINT fk_usage_authorizations_allowance_context
        FOREIGN KEY (
            usage_allowance_id,
            team_id,
            product,
            meter,
            billing_market,
            tier
        )
        REFERENCES usage_allowances (
            id,
            team_id,
            product,
            meter,
            billing_market,
            tier
        )
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
            route_type,
            tier,
            currency,
            unit_cost_units
        )
        REFERENCES sms_rates (
            id,
            billing_market,
            destination_country,
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
                AND route_type IN ('local', 'intl')
            )
            OR
            (
                product <> 'sms'
                AND destination_country IS NULL
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

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_team_product_meter_created
    ON usage_authorizations (
        team_id,
        product,
        meter,
        created_at DESC
    );

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_usage_allowance
    ON usage_authorizations (usage_allowance_id, created_at DESC)
    WHERE usage_allowance_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_sms_rate
    ON usage_authorizations (sms_rate_id, created_at DESC)
    WHERE sms_rate_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_authorizations_product_rate
    ON usage_authorizations (product_rate_id, created_at DESC)
    WHERE product_rate_id IS NOT NULL;
