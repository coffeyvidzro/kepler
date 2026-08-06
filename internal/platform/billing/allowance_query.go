package billing

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const activateDueWalletTierSQL = `
UPDATE team_wallets
SET tier = pending_tier,
    pending_tier = NULL,
    pending_tier_effective_at = NULL,
    updated_at = now()
WHERE team_id = $1
  AND pending_tier IS NOT NULL
  AND pending_tier_effective_at <= now()
`

const ensureCurrentAllowanceSQL = `
WITH clock AS MATERIALIZED (
    SELECT
        date_trunc('month', now() AT TIME ZONE 'UTC')
            AT TIME ZONE 'UTC' AS period_start,
        (
            date_trunc('month', now() AT TIME ZONE 'UTC')
            + interval '1 month'
        ) AT TIME ZONE 'UTC' AS period_end
),
wallet_record AS MATERIALIZED (
    SELECT wallet.team_id, wallet.billing_market, wallet.tier
    FROM team_wallets AS wallet
    WHERE wallet.team_id = $1
),
policy_record AS MATERIALIZED (
    SELECT policy.*
    FROM allowance_policies AS policy
    CROSS JOIN clock
    JOIN wallet_record AS wallet
      ON wallet.billing_market = policy.billing_market
     AND wallet.tier = policy.tier
    WHERE policy.product = $2
      AND policy.meter = $3
      AND policy.cadence = 'monthly'
      AND policy.effective_from <= clock.period_start
      AND (
          policy.effective_until IS NULL
          OR policy.effective_until > clock.period_start
      )
    ORDER BY policy.effective_from DESC
    LIMIT 1
)
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
WHERE NOT EXISTS (
    SELECT 1
    FROM usage_authorizations AS usage_auth
    WHERE usage_auth.team_id = wallet.team_id
      AND usage_auth.product = $2
      AND usage_auth.meter = $3
      AND usage_auth.reference_id = $4
)
ON CONFLICT (
    team_id,
    product,
    meter,
    period_start,
    period_end
) DO NOTHING
`

func prepareUsageAllowance(
	ctx context.Context,
	tx pgx.Tx,
	teamID uuid.UUID,
	product Product,
	meter string,
	referenceID string,
) error {
	if _, err := tx.Exec(ctx, activateDueWalletTierSQL, teamID); err != nil {
		return fmt.Errorf("activate due wallet tier: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		ensureCurrentAllowanceSQL,
		teamID,
		string(product),
		meter,
		referenceID,
	); err != nil {
		return fmt.Errorf("ensure current usage allowance: %w", err)
	}
	return nil
}
