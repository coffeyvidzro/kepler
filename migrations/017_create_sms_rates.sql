CREATE TABLE IF NOT EXISTS sms_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    billing_market CHAR(2) NOT NULL,
    destination_country CHAR(2) NOT NULL,
    provider TEXT NOT NULL,
    route_type TEXT NOT NULL DEFAULT 'standard',
    tier TEXT NOT NULL,
    currency CHAR(3) NOT NULL,
    cost_units BIGINT NOT NULL,

    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_sms_rates_audit
        UNIQUE (
            id,
            billing_market,
            destination_country,
            provider,
            route_type,
            tier,
            currency,
            cost_units
        ),

    CONSTRAINT fk_sms_rates_billing_market_currency
        FOREIGN KEY (billing_market, currency)
        REFERENCES billing_markets (code, currency)
        ON DELETE RESTRICT,

    CONSTRAINT chk_sms_rates_destination_country
        CHECK (destination_country ~ '^[A-Z]{2}$'),

    CONSTRAINT chk_sms_rates_provider
        CHECK (
            length(trim(provider)) > 0
            AND provider = lower(trim(provider))
            AND provider !~ '[[:space:]]'
        ),

    CONSTRAINT chk_sms_rates_route_type
        CHECK (
            length(trim(route_type)) > 0
            AND route_type = lower(trim(route_type))
            AND route_type !~ '[[:space:]]'
        ),

    CONSTRAINT chk_sms_rates_tier
        CHECK (tier IN ('growth', 'scale', 'enterprise')),

    CONSTRAINT chk_sms_rates_cost
        CHECK (cost_units > 0),

    CONSTRAINT chk_sms_rates_period
        CHECK (
            effective_until IS NULL
            OR effective_until > effective_from
        )
);

ALTER TABLE sms_rates
ADD CONSTRAINT ex_sms_rates_no_overlap
EXCLUDE USING gist (
    billing_market WITH =,
    destination_country WITH =,
    provider WITH =,
    route_type WITH =,
    tier WITH =,
    tstzrange(
        effective_from,
        COALESCE(effective_until, 'infinity'::timestamptz),
        '[)'
    ) WITH &&
);

CREATE INDEX IF NOT EXISTS idx_sms_rates_lookup
    ON sms_rates (
        billing_market,
        destination_country,
        provider,
        route_type,
        tier,
        effective_from DESC
    );
