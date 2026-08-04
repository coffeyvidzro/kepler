CREATE TABLE IF NOT EXISTS currencies (
    code CHAR(3) PRIMARY KEY,
    minor_unit SMALLINT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT true,

    CONSTRAINT chk_currencies_code
        CHECK (code ~ '^[A-Z]{3}$'),

    CONSTRAINT chk_currencies_minor_unit
        CHECK (minor_unit BETWEEN 0 AND 6)
);

INSERT INTO currencies (code, minor_unit, is_enabled)
VALUES
    ('GHS', 2, true),
    ('KES', 2, true)
ON CONFLICT (code) DO UPDATE SET
    minor_unit = EXCLUDED.minor_unit,
    is_enabled = EXCLUDED.is_enabled;

CREATE TABLE IF NOT EXISTS billing_markets (
    code CHAR(2) PRIMARY KEY,
    currency CHAR(3) NOT NULL
        REFERENCES currencies(code)
        ON DELETE RESTRICT,
    is_enabled BOOLEAN NOT NULL DEFAULT true,

    CONSTRAINT chk_billing_markets_code
        CHECK (code ~ '^[A-Z]{2}$'),

    CONSTRAINT uq_billing_markets_code_currency
        UNIQUE (code, currency)
);

INSERT INTO billing_markets (code, currency, is_enabled)
VALUES
    ('GH', 'GHS', true),
    ('KE', 'KES', true)
ON CONFLICT (code) DO UPDATE SET
    currency = EXCLUDED.currency,
    is_enabled = EXCLUDED.is_enabled;

CREATE TABLE IF NOT EXISTS team_wallets (
    team_id UUID PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,
    billing_market CHAR(2) NOT NULL,
    currency CHAR(3) NOT NULL,
    balance_units BIGINT NOT NULL DEFAULT 0,
    tier TEXT NOT NULL DEFAULT 'growth',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_team_wallets_team_market_currency
        UNIQUE (team_id, billing_market, currency),

    CONSTRAINT fk_team_wallets_billing_market_currency
        FOREIGN KEY (billing_market, currency)
        REFERENCES billing_markets (code, currency)
        ON DELETE RESTRICT,

    CONSTRAINT chk_team_wallets_balance
        CHECK (balance_units >= 0),

    CONSTRAINT chk_team_wallets_tier
        CHECK (tier IN ('growth', 'scale', 'enterprise'))
);

CREATE TABLE IF NOT EXISTS wallet_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE RESTRICT,
    usage_authorization_id UUID,
    amount_units BIGINT NOT NULL,
    transaction_type TEXT NOT NULL,
    reference_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT uq_wallet_ledger_reference
        UNIQUE (team_id, transaction_type, reference_id),

    CONSTRAINT chk_wallet_ledger_amount
        CHECK (amount_units <> 0),

    CONSTRAINT chk_wallet_ledger_transaction_type
        CHECK (
            transaction_type IN (
                'deposit',
                'usage',
                'refund',
                'expiry_wipe'
            )
        ),

    CONSTRAINT chk_wallet_ledger_usage_authorization
        CHECK (
            (
                transaction_type = 'usage'
                AND usage_authorization_id IS NOT NULL
                AND amount_units < 0
            )
            OR
            (
                transaction_type <> 'usage'
                AND usage_authorization_id IS NULL
            )
        ),

    CONSTRAINT chk_wallet_ledger_reference
        CHECK (length(trim(reference_id)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_wallet_ledger_team_created
    ON wallet_ledger (
        team_id,
        created_at DESC
    );
