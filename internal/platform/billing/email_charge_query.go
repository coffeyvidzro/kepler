package billing

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const chargeEmailUsageSQL = `
WITH clock AS MATERIALIZED (
    SELECT
        now() AS priced_at,
        date_trunc('month', now() AT TIME ZONE 'UTC')
            AT TIME ZONE 'UTC' AS period_start,
        (
            date_trunc('month', now() AT TIME ZONE 'UTC')
            + interval '1 month'
        ) AT TIME ZONE 'UTC' AS period_end
),
team_record AS MATERIALIZED (
    SELECT team.id, team.status, team.market_code
    FROM teams AS team
    WHERE team.id = $2
),
market_record AS MATERIALIZED (
    SELECT market.code, market.currency
    FROM billing_markets AS market
    JOIN team_record AS team ON team.market_code = market.code
    WHERE market.is_enabled = true
),
locked_wallet AS MATERIALIZED (
    SELECT wallet.*
    FROM team_wallets AS wallet
    WHERE wallet.team_id = $2
    FOR UPDATE
),
activated_wallet AS MATERIALIZED (
    UPDATE team_wallets AS wallet
    SET tier = locked.pending_tier,
        pending_tier = NULL,
        pending_tier_effective_at = NULL,
        updated_at = clock.priced_at
    FROM locked_wallet AS locked
    CROSS JOIN clock
    WHERE wallet.team_id = locked.team_id
      AND locked.pending_tier IS NOT NULL
      AND locked.pending_tier_effective_at <= clock.priced_at
    RETURNING wallet.*
),
wallet_record AS MATERIALIZED (
    SELECT *
    FROM activated_wallet
    UNION ALL
    SELECT *
    FROM locked_wallet
    WHERE NOT EXISTS (SELECT 1 FROM activated_wallet)
),
existing_charge AS MATERIALIZED (
    SELECT *
    FROM usage_authorizations AS usage_charge
    WHERE usage_charge.team_id = $2
      AND usage_charge.product = 'email'
      AND usage_charge.meter = 'email_recipient'
      AND usage_charge.reference_id = $3
),
policy_record AS MATERIALIZED (
    SELECT policy.*
    FROM allowance_policies AS policy
    CROSS JOIN clock
    JOIN wallet_record AS wallet
      ON wallet.billing_market = policy.billing_market
     AND wallet.tier = policy.tier
    WHERE policy.product = 'email'
      AND policy.meter = 'email_recipient'
      AND policy.cadence = 'monthly'
      AND policy.effective_from <= clock.period_start
      AND (
          policy.effective_until IS NULL
          OR policy.effective_until > clock.period_start
      )
    ORDER BY policy.effective_from DESC
    LIMIT 1
),
allowance_record AS MATERIALIZED (
    INSERT INTO usage_allowances (
        team_id,
        allowance_policy_id,
        product,
        meter,
        billing_market,
        tier,
        period_start,
        period_end,
        included_quantity
    )
    SELECT
        wallet.team_id,
        policy.id,
        policy.product,
        policy.meter,
        wallet.billing_market,
        wallet.tier,
        clock.period_start,
        clock.period_end,
        policy.included_quantity
    FROM wallet_record AS wallet
    CROSS JOIN clock
    JOIN policy_record AS policy
      ON policy.billing_market = wallet.billing_market
     AND policy.tier = wallet.tier
    WHERE NOT EXISTS (SELECT 1 FROM existing_charge)
    ON CONFLICT (
        team_id,
        product,
        meter,
        period_start,
        period_end
    ) DO UPDATE SET
        updated_at = usage_allowances.updated_at
    RETURNING *
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
        LEAST(
            $1::bigint,
            GREATEST(
                COALESCE(
                    allowance.included_quantity - allowance.consumed_quantity,
                    0
                ),
                0
            )
        )::bigint AS allowance_quantity,
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
        ($1::bigint - plan.allowance_quantity)::bigint AS billable_quantity,
        CASE
            WHEN $1::bigint - plan.allowance_quantity = 0
                THEN 0::bigint
            WHEN plan.unit_cost_units > 9223372036854775807 /
                NULLIF($1::bigint - plan.allowance_quantity, 0)
                THEN NULL::bigint
            ELSE plan.unit_cost_units *
                ($1::bigint - plan.allowance_quantity)
        END AS amount_units
    FROM plan
),
inserted_charge AS (
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
        $3,
        CASE WHEN plan.allowance_quantity > 0 THEN plan.usage_allowance_id END,
        CASE WHEN plan.billable_quantity > 0 THEN plan.product_rate_id END,
        plan.billing_market,
        $1,
        plan.allowance_quantity,
        plan.billable_quantity,
        CASE WHEN plan.billable_quantity > 0 THEN plan.unit_cost_units ELSE 0 END,
        plan.amount_units,
        plan.currency,
        plan.tier,
        plan.priced_at
    FROM priced_plan AS plan
    JOIN team_record AS team
      ON team.id = plan.team_id
     AND team.status = 'active'
    JOIN market_record AS market
      ON market.code = plan.billing_market
     AND market.currency = plan.currency
    WHERE $1::bigint > 0
      AND NOT EXISTS (SELECT 1 FROM existing_charge)
      AND plan.amount_units IS NOT NULL
      AND (plan.billable_quantity = 0 OR plan.product_rate_id IS NOT NULL)
      AND plan.balance_units >= plan.amount_units
    ON CONFLICT (team_id, product, meter, reference_id) DO NOTHING
    RETURNING *
),
updated_allowance AS (
    UPDATE usage_allowances AS allowance
    SET consumed_quantity = allowance.consumed_quantity
            + usage_charge.allowance_quantity,
        updated_at = now()
    FROM inserted_charge AS usage_charge
    WHERE allowance.id = usage_charge.usage_allowance_id
      AND usage_charge.allowance_quantity > 0
    RETURNING
        allowance.id,
        allowance.included_quantity,
        allowance.consumed_quantity
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
        usage_charge.team_id,
        usage_charge.id,
        -usage_charge.amount_units,
        'usage',
        usage_charge.reference_id
    FROM inserted_charge AS usage_charge
    WHERE usage_charge.amount_units > 0
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
resolved_charge AS MATERIALIZED (
    SELECT *
    FROM existing_charge
    UNION ALL
    SELECT *
    FROM inserted_charge
    LIMIT 1
)
SELECT
    CASE
        WHEN NOT EXISTS (SELECT 1 FROM team_record) THEN 'team_not_found'
        WHEN EXISTS (
            SELECT 1
            FROM team_record
            WHERE status <> 'active'
        ) THEN 'team_inactive'
        WHEN NOT EXISTS (SELECT 1 FROM market_record) THEN 'unsupported_market'
        WHEN NOT EXISTS (SELECT 1 FROM wallet_record) THEN 'wallet_not_found'
        WHEN EXISTS (SELECT 1 FROM existing_charge) THEN 'already_applied'
        WHEN EXISTS (
            SELECT 1
            FROM priced_plan
            WHERE billable_quantity > 0
              AND product_rate_id IS NULL
        ) THEN 'rate_not_found'
        WHEN EXISTS (
            SELECT 1
            FROM priced_plan
            WHERE amount_units IS NULL
        ) THEN 'amount_overflow'
        WHEN EXISTS (
            SELECT 1
            FROM priced_plan
            WHERE amount_units IS NOT NULL
              AND balance_units < amount_units
        ) THEN 'insufficient_balance'
        WHEN EXISTS (
            SELECT 1
            FROM inserted_charge
            WHERE allowance_quantity = total_quantity
        ) THEN 'allowance_applied'
        WHEN EXISTS (SELECT 1 FROM inserted_charge) THEN 'applied'
        ELSE 'already_applied'
    END AS outcome,
    COALESCE(
        (SELECT billing_market FROM resolved_charge),
        (SELECT billing_market FROM wallet_record),
        ''
    )::text AS market_code,
    COALESCE(
        (SELECT currency FROM resolved_charge),
        (SELECT currency FROM wallet_record),
        ''
    )::text AS currency,
    COALESCE(
        (SELECT tier FROM resolved_charge),
        (SELECT tier FROM wallet_record),
        ''
    )::text AS tier,
    'email'::text AS product,
    COALESCE(
        (SELECT unit_cost_units FROM resolved_charge),
        0
    )::bigint AS unit_cost_units,
    COALESCE(
        (SELECT total_quantity FROM resolved_charge),
        $1::bigint
    )::bigint AS quantity,
    COALESCE(
        (SELECT amount_units FROM resolved_charge),
        0
    )::bigint AS amount_units,
    COALESCE(
        (SELECT balance_units FROM updated_wallet),
        (SELECT balance_units FROM wallet_record),
        0
    )::bigint AS balance_units,
    COALESCE(
        (SELECT allowance_quantity > 0 FROM resolved_charge),
        false
    )::boolean AS covered_by_allowance,
    COALESCE(
        (SELECT included_quantity - consumed_quantity FROM updated_allowance),
        (SELECT included_quantity - consumed_quantity FROM allowance_record),
        0
    )::bigint AS remaining_allowance
`

func chargeEmailUsage(
	ctx context.Context,
	tx pgx.Tx,
	input EmailChargeInput,
) (Charge, error) {
	var outcome string
	var product string
	var charge Charge
	err := tx.QueryRow(
		ctx,
		chargeEmailUsageSQL,
		input.RecipientCount,
		input.TeamID,
		input.MessageID.String(),
	).Scan(
		&outcome,
		&charge.MarketCode,
		&charge.Currency,
		&charge.Tier,
		&product,
		&charge.UnitCostUnits,
		&charge.Quantity,
		&charge.AmountUnits,
		&charge.RemainingBalance,
		&charge.CoveredByAllowance,
		&charge.RemainingAllowance,
	)
	charge.Outcome = Outcome(outcome)
	charge.Product = Product(product)
	return charge, err
}
