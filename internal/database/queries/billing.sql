-- name: GetTeamWallet :one
SELECT *
FROM team_wallets
WHERE team_id = sqlc.arg(team_id);

-- name: GetActiveProductRate :one
SELECT *
FROM product_rates
WHERE product = sqlc.arg(product)
  AND meter = sqlc.arg(meter)
  AND billing_market = sqlc.arg(billing_market)
  AND tier = sqlc.arg(tier)
  AND effective_from <= sqlc.arg(priced_at)
  AND (effective_until IS NULL OR effective_until > sqlc.arg(priced_at))
ORDER BY effective_from DESC
LIMIT 1;

-- name: ListWalletLedger :many
SELECT *
FROM wallet_ledger
WHERE team_id = sqlc.arg(team_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count)
OFFSET sqlc.arg(offset_count);

-- name: CreditTeamWallet :one
WITH locked_wallet AS MATERIALIZED (
    SELECT wallet.*
    FROM team_wallets AS wallet
    WHERE wallet.team_id = sqlc.arg(team_id)
    FOR UPDATE
),
inserted_ledger AS (
    INSERT INTO wallet_ledger (
        team_id,
        amount_units,
        transaction_type,
        reference_id
    )
    SELECT
        wallet.team_id,
        sqlc.arg(amount_units),
        sqlc.arg(transaction_type),
        sqlc.arg(reference_id)
    FROM locked_wallet AS wallet
    WHERE sqlc.arg(amount_units)::bigint > 0
      AND sqlc.arg(transaction_type)::text <> 'usage'
    ON CONFLICT (team_id, transaction_type, reference_id) DO NOTHING
    RETURNING team_id, amount_units
),
updated_wallet AS (
    UPDATE team_wallets AS wallet
    SET balance_units = wallet.balance_units + ledger.amount_units,
        updated_at = now()
    FROM inserted_ledger AS ledger
    WHERE wallet.team_id = ledger.team_id
    RETURNING wallet.*
)
SELECT *
FROM updated_wallet;

-- name: DebitTeamWallet :one
WITH locked_wallet AS MATERIALIZED (
    SELECT wallet.*
    FROM team_wallets AS wallet
    WHERE wallet.team_id = sqlc.arg(team_id)
    FOR UPDATE
),
inserted_ledger AS (
    INSERT INTO wallet_ledger (
        team_id,
        amount_units,
        transaction_type,
        reference_id
    )
    SELECT
        wallet.team_id,
        -sqlc.arg(amount_units)::bigint,
        sqlc.arg(transaction_type),
        sqlc.arg(reference_id)
    FROM locked_wallet AS wallet
    WHERE sqlc.arg(amount_units)::bigint > 0
      AND sqlc.arg(transaction_type)::text <> 'usage'
      AND wallet.balance_units >= sqlc.arg(amount_units)
    ON CONFLICT (team_id, transaction_type, reference_id) DO NOTHING
    RETURNING team_id, amount_units
),
updated_wallet AS (
    UPDATE team_wallets AS wallet
    SET balance_units = wallet.balance_units + ledger.amount_units,
        updated_at = now()
    FROM inserted_ledger AS ledger
    WHERE wallet.team_id = ledger.team_id
    RETURNING wallet.*
)
SELECT *
FROM updated_wallet;

-- name: UpdateTeamWalletTier :one
UPDATE team_wallets
SET tier = sqlc.arg(tier),
    updated_at = now()
WHERE team_id = sqlc.arg(team_id)
RETURNING *;

-- name: AuthorizeSMSCharge :one
WITH clock AS MATERIALIZED (
    SELECT now() AS priced_at
),
team_record AS MATERIALIZED (
    SELECT team.id, team.status, team.market_code
    FROM teams AS team
    WHERE team.id = sqlc.arg(team_id)
),
market_record AS MATERIALIZED (
    SELECT market.code, market.currency
    FROM billing_markets AS market
    JOIN team_record AS team ON team.market_code = market.code
    WHERE market.is_enabled = true
),
wallet_record AS MATERIALIZED (
    SELECT wallet.*
    FROM team_wallets AS wallet
    WHERE wallet.team_id = sqlc.arg(team_id)
    FOR UPDATE
),
existing_authorization AS MATERIALIZED (
    SELECT *
    FROM usage_authorizations AS usage_auth
    WHERE usage_auth.team_id = sqlc.arg(team_id)
      AND usage_auth.meter = 'sms_segment'
      AND usage_auth.reference_id = sqlc.arg(reference_id)
),
allowance_record AS MATERIALIZED (
    SELECT allowance.*
    FROM usage_allowances AS allowance
    CROSS JOIN clock
    WHERE allowance.team_id = sqlc.arg(team_id)
      AND allowance.meter = 'sms_segment'
      AND allowance.period_start <= clock.priced_at
      AND allowance.period_end > clock.priced_at
      AND allowance.consumed_quantity < allowance.included_quantity
    ORDER BY allowance.period_start DESC
    LIMIT 1
    FOR UPDATE
),
rate_record AS MATERIALIZED (
    SELECT rate.*
    FROM sms_rates AS rate
    CROSS JOIN clock
    JOIN wallet_record AS wallet
      ON wallet.billing_market = rate.billing_market
     AND wallet.currency = rate.currency
     AND wallet.tier = rate.tier
    WHERE rate.destination_country = sqlc.arg(destination_country)
      AND rate.provider = sqlc.arg(provider)
      AND rate.route_type = sqlc.arg(route_type)
      AND rate.effective_from <= clock.priced_at
      AND (rate.effective_until IS NULL OR rate.effective_until > clock.priced_at)
    ORDER BY rate.effective_from DESC
    LIMIT 1
),
plan AS MATERIALIZED (
    SELECT
        wallet.team_id,
        wallet.billing_market,
        wallet.currency,
        wallet.tier,
        wallet.balance_units,
        allowance.id AS usage_allowance_id,
        LEAST(
            sqlc.arg(quantity)::bigint,
            GREATEST(
                COALESCE(allowance.included_quantity - allowance.consumed_quantity, 0),
                0
            )
        )::bigint AS allowance_quantity,
        rate.id AS sms_rate_id,
        COALESCE(rate.cost_units, 0)::bigint AS unit_cost_units,
        clock.priced_at
    FROM wallet_record AS wallet
    CROSS JOIN clock
    LEFT JOIN allowance_record AS allowance ON true
    LEFT JOIN rate_record AS rate ON true
),
priced_plan AS MATERIALIZED (
    SELECT
        plan.*,
        (sqlc.arg(quantity)::bigint - plan.allowance_quantity)::bigint AS billable_quantity,
        CASE
            WHEN sqlc.arg(quantity)::bigint - plan.allowance_quantity = 0 THEN 0::bigint
            WHEN plan.unit_cost_units > 9223372036854775807 /
                NULLIF(sqlc.arg(quantity)::bigint - plan.allowance_quantity, 0)
                THEN NULL::bigint
            ELSE plan.unit_cost_units *
                (sqlc.arg(quantity)::bigint - plan.allowance_quantity)
        END AS amount_units
    FROM plan
),
inserted_authorization AS (
    INSERT INTO usage_authorizations (
        team_id,
        product,
        meter,
        reference_id,
        usage_allowance_id,
        sms_rate_id,
        billing_market,
        destination_country,
        provider,
        route_type,
        total_quantity,
        allowance_quantity,
        billable_quantity,
        unit_cost_units,
        amount_units,
        currency,
        tier,
        priced_at
    )
    SELECT
        plan.team_id,
        'sms',
        'sms_segment',
        sqlc.arg(reference_id),
        CASE WHEN plan.allowance_quantity > 0 THEN plan.usage_allowance_id END,
        CASE WHEN plan.billable_quantity > 0 THEN plan.sms_rate_id END,
        plan.billing_market,
        sqlc.arg(destination_country),
        sqlc.arg(provider),
        sqlc.arg(route_type),
        sqlc.arg(quantity),
        plan.allowance_quantity,
        plan.billable_quantity,
        CASE WHEN plan.billable_quantity > 0 THEN plan.unit_cost_units ELSE 0 END,
        plan.amount_units,
        plan.currency,
        plan.tier,
        plan.priced_at
    FROM priced_plan AS plan
    JOIN team_record AS team ON team.id = plan.team_id AND team.status = 'active'
    JOIN market_record AS market ON market.code = plan.billing_market AND market.currency = plan.currency
    WHERE sqlc.arg(quantity)::bigint > 0
      AND NOT EXISTS (SELECT 1 FROM existing_authorization)
      AND plan.amount_units IS NOT NULL
      AND (plan.billable_quantity = 0 OR plan.sms_rate_id IS NOT NULL)
      AND plan.balance_units >= plan.amount_units
    ON CONFLICT (team_id, meter, reference_id) DO NOTHING
    RETURNING *
),
updated_allowance AS (
    UPDATE usage_allowances AS allowance
    SET consumed_quantity = allowance.consumed_quantity + usage_auth.allowance_quantity,
        updated_at = now()
    FROM inserted_authorization AS usage_auth
    WHERE allowance.id = usage_auth.usage_allowance_id
      AND usage_auth.allowance_quantity > 0
    RETURNING allowance.id, allowance.included_quantity, allowance.consumed_quantity
),
inserted_ledger AS (
    INSERT INTO wallet_ledger (
        team_id,
        usage_authorization_id,
        amount_units,
        transaction_type,
        reference_id
    )
    SELECT
        usage_auth.team_id,
        usage_auth.id,
        -usage_auth.amount_units,
        'usage',
        usage_auth.reference_id
    FROM inserted_authorization AS usage_auth
    WHERE usage_auth.amount_units > 0
    RETURNING team_id, amount_units
),
updated_wallet AS (
    UPDATE team_wallets AS wallet
    SET balance_units = wallet.balance_units + ledger.amount_units,
        updated_at = now()
    FROM inserted_ledger AS ledger
    WHERE wallet.team_id = ledger.team_id
    RETURNING wallet.balance_units
),
resolved_authorization AS MATERIALIZED (
    SELECT * FROM existing_authorization
    UNION ALL
    SELECT * FROM inserted_authorization
    LIMIT 1
)
SELECT
    CASE
        WHEN NOT EXISTS (SELECT 1 FROM team_record) THEN 'team_not_found'
        WHEN EXISTS (SELECT 1 FROM team_record WHERE status <> 'active') THEN 'team_inactive'
        WHEN NOT EXISTS (SELECT 1 FROM market_record) THEN 'unsupported_market'
        WHEN NOT EXISTS (SELECT 1 FROM wallet_record) THEN 'wallet_not_found'
        WHEN EXISTS (SELECT 1 FROM existing_authorization) THEN 'already_applied'
        WHEN EXISTS (
            SELECT 1 FROM priced_plan
            WHERE billable_quantity > 0 AND sms_rate_id IS NULL
        ) THEN 'rate_not_found'
        WHEN EXISTS (SELECT 1 FROM priced_plan WHERE amount_units IS NULL) THEN 'amount_overflow'
        WHEN EXISTS (
            SELECT 1 FROM priced_plan
            WHERE amount_units IS NOT NULL AND balance_units < amount_units
        ) THEN 'insufficient_balance'
        WHEN EXISTS (
            SELECT 1 FROM inserted_authorization
            WHERE allowance_quantity = total_quantity
        ) THEN 'allowance_applied'
        WHEN EXISTS (SELECT 1 FROM inserted_authorization) THEN 'applied'
        ELSE 'already_applied'
    END AS outcome,
    COALESCE((SELECT billing_market FROM resolved_authorization), (SELECT billing_market FROM wallet_record), '')::text AS market_code,
    COALESCE((SELECT currency FROM resolved_authorization), (SELECT currency FROM wallet_record), '')::text AS currency,
    COALESCE((SELECT tier FROM resolved_authorization), (SELECT tier FROM wallet_record), '')::text AS tier,
    'sms'::text AS product,
    COALESCE((SELECT unit_cost_units FROM resolved_authorization), 0)::bigint AS unit_cost_units,
    sqlc.arg(quantity)::bigint AS quantity,
    COALESCE((SELECT amount_units FROM resolved_authorization), 0)::bigint AS amount_units,
    COALESCE((SELECT balance_units FROM updated_wallet), (SELECT balance_units FROM wallet_record), 0)::bigint AS balance_units,
    COALESCE((SELECT allowance_quantity > 0 FROM resolved_authorization), false)::boolean AS covered_by_allowance,
    COALESCE(
        (SELECT included_quantity - consumed_quantity FROM updated_allowance),
        (SELECT included_quantity - consumed_quantity FROM allowance_record),
        0
    )::bigint AS remaining_allowance;

-- name: AuthorizeEmailCharge :one
WITH clock AS MATERIALIZED (
    SELECT now() AS priced_at
),
team_record AS MATERIALIZED (
    SELECT team.id, team.status, team.market_code
    FROM teams AS team
    WHERE team.id = sqlc.arg(team_id)
),
market_record AS MATERIALIZED (
    SELECT market.code, market.currency
    FROM billing_markets AS market
    JOIN team_record AS team ON team.market_code = market.code
    WHERE market.is_enabled = true
),
wallet_record AS MATERIALIZED (
    SELECT wallet.*
    FROM team_wallets AS wallet
    WHERE wallet.team_id = sqlc.arg(team_id)
    FOR UPDATE
),
existing_authorization AS MATERIALIZED (
    SELECT *
    FROM usage_authorizations AS usage_auth
    WHERE usage_auth.team_id = sqlc.arg(team_id)
      AND usage_auth.meter = 'email_recipient'
      AND usage_auth.reference_id = sqlc.arg(reference_id)
),
allowance_record AS MATERIALIZED (
    SELECT allowance.*
    FROM usage_allowances AS allowance
    CROSS JOIN clock
    WHERE allowance.team_id = sqlc.arg(team_id)
      AND allowance.meter = 'email_recipient'
      AND allowance.period_start <= clock.priced_at
      AND allowance.period_end > clock.priced_at
      AND allowance.consumed_quantity < allowance.included_quantity
    ORDER BY allowance.period_start DESC
    LIMIT 1
    FOR UPDATE
),
rate_record AS MATERIALIZED (
    SELECT rate.*
    FROM product_rates AS rate
    CROSS JOIN clock
    JOIN wallet_record AS wallet
      ON wallet.billing_market = rate.billing_market
     AND wallet.currency = rate.currency
     AND wallet.tier = rate.tier
    WHERE rate.product = 'email'
      AND rate.meter = 'email_recipient'
      AND rate.effective_from <= clock.priced_at
      AND (rate.effective_until IS NULL OR rate.effective_until > clock.priced_at)
    ORDER BY rate.effective_from DESC
    LIMIT 1
),
plan AS MATERIALIZED (
    SELECT
        wallet.team_id,
        wallet.billing_market,
        wallet.currency,
        wallet.tier,
        wallet.balance_units,
        allowance.id AS usage_allowance_id,
        CASE WHEN allowance.id IS NULL THEN 0::bigint ELSE 1::bigint END AS allowance_quantity,
        rate.id AS product_rate_id,
        COALESCE(rate.cost_units, 0)::bigint AS unit_cost_units,
        clock.priced_at
    FROM wallet_record AS wallet
    CROSS JOIN clock
    LEFT JOIN allowance_record AS allowance ON true
    LEFT JOIN rate_record AS rate ON true
),
priced_plan AS MATERIALIZED (
    SELECT
        plan.*,
        (1 - plan.allowance_quantity)::bigint AS billable_quantity,
        CASE WHEN plan.allowance_quantity = 1 THEN 0::bigint ELSE plan.unit_cost_units END AS amount_units
    FROM plan
),
inserted_authorization AS (
    INSERT INTO usage_authorizations (
        team_id,
        product,
        meter,
        reference_id,
        usage_allowance_id,
        product_rate_id,
        billing_market,
        total_quantity,
        allowance_quantity,
        billable_quantity,
        unit_cost_units,
        amount_units,
        currency,
        tier,
        priced_at
    )
    SELECT
        plan.team_id,
        'email',
        'email_recipient',
        sqlc.arg(reference_id),
        CASE WHEN plan.allowance_quantity > 0 THEN plan.usage_allowance_id END,
        CASE WHEN plan.billable_quantity > 0 THEN plan.product_rate_id END,
        plan.billing_market,
        1,
        plan.allowance_quantity,
        plan.billable_quantity,
        CASE WHEN plan.billable_quantity > 0 THEN plan.unit_cost_units ELSE 0 END,
        plan.amount_units,
        plan.currency,
        plan.tier,
        plan.priced_at
    FROM priced_plan AS plan
    JOIN team_record AS team ON team.id = plan.team_id AND team.status = 'active'
    JOIN market_record AS market ON market.code = plan.billing_market AND market.currency = plan.currency
    WHERE NOT EXISTS (SELECT 1 FROM existing_authorization)
      AND (plan.billable_quantity = 0 OR plan.product_rate_id IS NOT NULL)
      AND plan.balance_units >= plan.amount_units
    ON CONFLICT (team_id, meter, reference_id) DO NOTHING
    RETURNING *
),
updated_allowance AS (
    UPDATE usage_allowances AS allowance
    SET consumed_quantity = allowance.consumed_quantity + usage_auth.allowance_quantity,
        updated_at = now()
    FROM inserted_authorization AS usage_auth
    WHERE allowance.id = usage_auth.usage_allowance_id
      AND usage_auth.allowance_quantity > 0
    RETURNING allowance.id, allowance.included_quantity, allowance.consumed_quantity
),
inserted_ledger AS (
    INSERT INTO wallet_ledger (
        team_id,
        usage_authorization_id,
        amount_units,
        transaction_type,
        reference_id
    )
    SELECT
        usage_auth.team_id,
        usage_auth.id,
        -usage_auth.amount_units,
        'usage',
        usage_auth.reference_id
    FROM inserted_authorization AS usage_auth
    WHERE usage_auth.amount_units > 0
    RETURNING team_id, amount_units
),
updated_wallet AS (
    UPDATE team_wallets AS wallet
    SET balance_units = wallet.balance_units + ledger.amount_units,
        updated_at = now()
    FROM inserted_ledger AS ledger
    WHERE wallet.team_id = ledger.team_id
    RETURNING wallet.balance_units
),
resolved_authorization AS MATERIALIZED (
    SELECT * FROM existing_authorization
    UNION ALL
    SELECT * FROM inserted_authorization
    LIMIT 1
)
SELECT
    CASE
        WHEN NOT EXISTS (SELECT 1 FROM team_record) THEN 'team_not_found'
        WHEN EXISTS (SELECT 1 FROM team_record WHERE status <> 'active') THEN 'team_inactive'
        WHEN NOT EXISTS (SELECT 1 FROM market_record) THEN 'unsupported_market'
        WHEN NOT EXISTS (SELECT 1 FROM wallet_record) THEN 'wallet_not_found'
        WHEN EXISTS (SELECT 1 FROM existing_authorization) THEN 'already_applied'
        WHEN EXISTS (
            SELECT 1 FROM priced_plan
            WHERE billable_quantity > 0 AND product_rate_id IS NULL
        ) THEN 'rate_not_found'
        WHEN EXISTS (
            SELECT 1 FROM priced_plan
            WHERE balance_units < amount_units
        ) THEN 'insufficient_balance'
        WHEN EXISTS (
            SELECT 1 FROM inserted_authorization
            WHERE allowance_quantity = total_quantity
        ) THEN 'allowance_applied'
        WHEN EXISTS (SELECT 1 FROM inserted_authorization) THEN 'applied'
        ELSE 'already_applied'
    END AS outcome,
    COALESCE((SELECT billing_market FROM resolved_authorization), (SELECT billing_market FROM wallet_record), '')::text AS market_code,
    COALESCE((SELECT currency FROM resolved_authorization), (SELECT currency FROM wallet_record), '')::text AS currency,
    COALESCE((SELECT tier FROM resolved_authorization), (SELECT tier FROM wallet_record), '')::text AS tier,
    'email'::text AS product,
    COALESCE((SELECT unit_cost_units FROM resolved_authorization), 0)::bigint AS unit_cost_units,
    1::bigint AS quantity,
    COALESCE((SELECT amount_units FROM resolved_authorization), 0)::bigint AS amount_units,
    COALESCE((SELECT balance_units FROM updated_wallet), (SELECT balance_units FROM wallet_record), 0)::bigint AS balance_units,
    COALESCE((SELECT allowance_quantity > 0 FROM resolved_authorization), false)::boolean AS covered_by_allowance,
    COALESCE(
        (SELECT included_quantity - consumed_quantity FROM updated_allowance),
        (SELECT included_quantity - consumed_quantity FROM allowance_record),
        0
    )::bigint AS remaining_allowance;
