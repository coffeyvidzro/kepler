CREATE TABLE IF NOT EXISTS product_rates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    product TEXT NOT NULL,
    meter TEXT NOT NULL,
    billing_market CHAR(2) NOT NULL,
    tier TEXT NOT NULL,
    currency CHAR(3) NOT NULL,
    cost_units BIGINT NOT NULL,

    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_product_rates_audit
        UNIQUE (
            id,
            product,
            meter,
            billing_market,
            tier,
            currency,
            cost_units
        ),

    CONSTRAINT fk_product_rates_billing_market_currency
        FOREIGN KEY (billing_market, currency)
        REFERENCES billing_markets (code, currency)
        ON DELETE RESTRICT,

    CONSTRAINT chk_product_rates_product
        CHECK (
            length(trim(product)) > 0
            AND product = lower(trim(product))
            AND product !~ '[[:space:]]'
        ),

    CONSTRAINT chk_product_rates_meter
        CHECK (
            length(trim(meter)) > 0
            AND meter = lower(trim(meter))
            AND meter !~ '[[:space:]]'
        ),

    CONSTRAINT chk_product_rates_tier
        CHECK (tier IN ('growth', 'scale', 'enterprise')),

    CONSTRAINT chk_product_rates_cost
        CHECK (cost_units > 0),

    CONSTRAINT chk_product_rates_period
        CHECK (
            effective_until IS NULL
            OR effective_until > effective_from
        )
);

ALTER TABLE product_rates
ADD CONSTRAINT ex_product_rates_no_overlap
EXCLUDE USING gist (
    product WITH =,
    meter WITH =,
    billing_market WITH =,
    tier WITH =,
    tstzrange(
        effective_from,
        COALESCE(effective_until, 'infinity'::timestamptz),
        '[)'
    ) WITH &&
);

CREATE INDEX IF NOT EXISTS idx_product_rates_lookup
    ON product_rates (
        product,
        meter,
        billing_market,
        tier,
        effective_from DESC
    );
