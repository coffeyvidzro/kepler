package messaging_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	"github.com/coffeyvidzro/dugble/server/internal/platform/billing"
)

func TestCreateTeamDoesNotPregrantAllowance(t *testing.T) {
	pool := openFreshDatabase(t)
	seedBillingMarket(t, pool)

	ctx := context.Background()
	ownerID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, name)
		VALUES ($1, 'allowance-owner@example.com', 'Allowance Owner')
	`, ownerID); err != nil {
		t.Fatalf("seed owner: %v", err)
	}

	team, err := dbsqlc.New(pool).CreateTeamWithOwner(
		ctx,
		dbsqlc.CreateTeamWithOwnerParams{
			Name:       "No Pregrant Team",
			MarketCode: "GH",
			Phone:      "+233200000001",
			Address:    "Accra",
			OwnerID:    &ownerID,
		},
	)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if count := requireCount(
		t,
		pool,
		`SELECT count(*) FROM usage_allowances WHERE team_id = $1`,
		team.ID,
	); count != 0 {
		t.Fatalf("expected no pre-granted allowance, got %d", count)
	}
	if count := requireCount(
		t,
		pool,
		`SELECT count(*) FROM team_wallets WHERE team_id = $1`,
		team.ID,
	); count != 1 {
		t.Fatalf("expected one wallet, got %d", count)
	}
}

func TestEmailAllowanceIsGeneratedAndAuthorizationIsIdempotent(t *testing.T) {
	pool := openFreshDatabase(t)
	seedBillingMarket(t, pool)
	teamID := seedBillingTeam(t, pool, "growth", 1_000)
	seedEmailRate(t, pool, "growth", 5)
	policyID := seedAllowancePolicy(
		t,
		pool,
		"email",
		"email_recipient",
		"growth",
		100,
	)

	messageID := uuid.New()
	charge := chargeEmail(t, pool, billing.EmailChargeInput{
		TeamID:         teamID,
		MessageID:      messageID,
		RecipientCount: 120,
	})
	if charge.Outcome != billing.OutcomeApplied {
		t.Fatalf("expected applied, got %q", charge.Outcome)
	}
	if charge.AmountUnits != 100 || charge.RemainingBalance != 900 {
		t.Fatalf(
			"unexpected charge amount=%d balance=%d",
			charge.AmountUnits,
			charge.RemainingBalance,
		)
	}
	if !charge.CoveredByAllowance || charge.RemainingAllowance != 0 {
		t.Fatalf(
			"unexpected allowance result covered=%t remaining=%d",
			charge.CoveredByAllowance,
			charge.RemainingAllowance,
		)
	}

	ctx := context.Background()
	var (
		storedPolicyID uuid.UUID
		product        string
		meter          string
		market         string
		tier           string
		included       int64
		consumed       int64
		isUTCMonth     bool
	)
	if err := pool.QueryRow(ctx, `
		SELECT
			allowance_policy_id,
			product,
			meter,
			billing_market,
			tier,
			included_quantity,
			consumed_quantity,
			period_start = (
				date_trunc('month', period_start AT TIME ZONE 'UTC')
				AT TIME ZONE 'UTC'
			)
			AND period_end = (
				date_trunc('month', period_start AT TIME ZONE 'UTC')
				+ interval '1 month'
			) AT TIME ZONE 'UTC'
		FROM usage_allowances
		WHERE team_id = $1
	`, teamID).Scan(
		&storedPolicyID,
		&product,
		&meter,
		&market,
		&tier,
		&included,
		&consumed,
		&isUTCMonth,
	); err != nil {
		t.Fatalf("read generated allowance: %v", err)
	}
	if storedPolicyID != policyID {
		t.Fatalf("expected policy %s, got %s", policyID, storedPolicyID)
	}
	if product != "email" || meter != "email_recipient" {
		t.Fatalf("unexpected product/meter %s/%s", product, meter)
	}
	if market != "GH" || tier != "growth" {
		t.Fatalf("unexpected market/tier %s/%s", market, tier)
	}
	if included != 100 || consumed != 100 || !isUTCMonth {
		t.Fatalf(
			"unexpected grant included=%d consumed=%d utc_month=%t",
			included,
			consumed,
			isUTCMonth,
		)
	}

	retry := chargeEmail(t, pool, billing.EmailChargeInput{
		TeamID:         teamID,
		MessageID:      messageID,
		RecipientCount: 120,
	})
	if retry.Outcome != billing.OutcomeAlreadyApplied {
		t.Fatalf("expected already_applied, got %q", retry.Outcome)
	}
	if retry.AmountUnits != 100 || retry.RemainingBalance != 900 {
		t.Fatalf(
			"retry changed state amount=%d balance=%d",
			retry.AmountUnits,
			retry.RemainingBalance,
		)
	}
	if count := requireCount(
		t,
		pool,
		`SELECT count(*) FROM usage_authorizations WHERE team_id = $1`,
		teamID,
	); count != 1 {
		t.Fatalf("expected one immutable authorization, got %d", count)
	}
}

func TestMissingAllowancePolicyMeansFullyBillable(t *testing.T) {
	pool := openFreshDatabase(t)
	seedBillingMarket(t, pool)
	teamID := seedBillingTeam(t, pool, "growth", 1_000)
	seedEmailRate(t, pool, "growth", 5)

	charge := chargeEmail(t, pool, billing.EmailChargeInput{
		TeamID:         teamID,
		MessageID:      uuid.New(),
		RecipientCount: 10,
	})
	if charge.Outcome != billing.OutcomeApplied {
		t.Fatalf("expected applied, got %q", charge.Outcome)
	}
	if charge.CoveredByAllowance || charge.RemainingAllowance != 0 {
		t.Fatalf(
			"unexpected allowance result covered=%t remaining=%d",
			charge.CoveredByAllowance,
			charge.RemainingAllowance,
		)
	}
	if charge.AmountUnits != 50 || charge.RemainingBalance != 950 {
		t.Fatalf(
			"unexpected charge amount=%d balance=%d",
			charge.AmountUnits,
			charge.RemainingBalance,
		)
	}
	if count := requireCount(
		t,
		pool,
		`SELECT count(*) FROM usage_allowances WHERE team_id = $1`,
		teamID,
	); count != 0 {
		t.Fatalf("expected no allowance row, got %d", count)
	}
}

func TestSMSAllowanceSplitsUsageAndCharge(t *testing.T) {
	pool := openFreshDatabase(t)
	seedBillingMarket(t, pool)
	teamID := seedBillingTeam(t, pool, "growth", 1_000)

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO sms_rates (
			billing_market,
			destination_country,
			route_type,
			tier,
			currency,
			cost_units,
			effective_from
		)
		VALUES ('GH', 'GH', 'local', 'growth', 'GHS', 10, now() - interval '1 day')
	`); err != nil {
		t.Fatalf("seed SMS rate: %v", err)
	}
	seedAllowancePolicy(t, pool, "sms", "sms_segment", "growth", 2)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin SMS transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := dbsqlc.New(pool).WithTx(tx).AuthorizeSMSCharge(
		ctx,
		dbsqlc.AuthorizeSMSChargeParams{
			Quantity:           5,
			TeamID:             teamID,
			DestinationCountry: "GH",
			ReferenceID:        uuid.NewString(),
		},
	)
	if err != nil {
		t.Fatalf("authorize SMS charge: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit SMS transaction: %v", err)
	}

	if row.Outcome != "applied" || !row.CoveredByAllowance {
		t.Fatalf(
			"unexpected SMS outcome=%q covered=%t",
			row.Outcome,
			row.CoveredByAllowance,
		)
	}
	if row.AmountUnits != 30 || row.BalanceUnits != 970 {
		t.Fatalf(
			"unexpected SMS amount=%d balance=%d",
			row.AmountUnits,
			row.BalanceUnits,
		)
	}
	if got := requireString(
		t,
		pool,
		`SELECT product || ':' || meter FROM usage_allowances WHERE team_id = $1`,
		teamID,
	); got != "sms:sms_segment" {
		t.Fatalf("unexpected SMS allowance %q", got)
	}
}

func TestDuePendingTierActivatesBeforeAllowanceGeneration(t *testing.T) {
	pool := openFreshDatabase(t)
	seedBillingMarket(t, pool)
	teamID := seedBillingTeam(t, pool, "growth", 1_000)
	seedEmailRate(t, pool, "scale", 7)
	seedAllowancePolicy(t, pool, "email", "email_recipient", "scale", 50)

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE team_wallets
		SET pending_tier = 'scale',
			pending_tier_effective_at =
				date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		WHERE team_id = $1
	`, teamID); err != nil {
		t.Fatalf("make pending tier due: %v", err)
	}

	charge := chargeEmail(t, pool, billing.EmailChargeInput{
		TeamID:         teamID,
		MessageID:      uuid.New(),
		RecipientCount: 20,
	})
	if charge.Outcome != billing.OutcomeAllowanceApplied {
		t.Fatalf("expected allowance_applied, got %q", charge.Outcome)
	}
	if charge.Tier != "scale" || charge.AmountUnits != 0 {
		t.Fatalf(
			"unexpected activated tier=%q amount=%d",
			charge.Tier,
			charge.AmountUnits,
		)
	}
	if charge.RemainingAllowance != 30 {
		t.Fatalf("expected 30 allowance remaining, got %d", charge.RemainingAllowance)
	}

	var (
		walletTier  string
		pendingTier *string
	)
	if err := pool.QueryRow(ctx, `
		SELECT tier, pending_tier
		FROM team_wallets
		WHERE team_id = $1
	`, teamID).Scan(&walletTier, &pendingTier); err != nil {
		t.Fatalf("read wallet tier: %v", err)
	}
	if walletTier != "scale" || pendingTier != nil {
		t.Fatalf("unexpected wallet tier=%q pending=%v", walletTier, pendingTier)
	}
	if got := requireString(
		t,
		pool,
		`SELECT tier FROM usage_allowances WHERE team_id = $1`,
		teamID,
	); got != "scale" {
		t.Fatalf("expected scale allowance, got %q", got)
	}
}

func seedBillingMarket(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO currencies (code, minor_unit, is_enabled)
		VALUES ('GHS', 2, true)
	`); err != nil {
		t.Fatalf("seed currency: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO billing_markets (code, currency, is_enabled)
		VALUES ('GH', 'GHS', true)
	`); err != nil {
		t.Fatalf("seed billing market: %v", err)
	}
}

func seedBillingTeam(
	t *testing.T,
	pool *pgxpool.Pool,
	tier string,
	balance int64,
) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	teamID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO teams (
			id,
			name,
			market_code,
			phone,
			address,
			status
		)
		VALUES ($1, 'Billing Test', 'GH', '+233200000002', 'Accra', 'active')
	`, teamID); err != nil {
		t.Fatalf("seed billing team: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO team_wallets (
			team_id,
			billing_market,
			currency,
			balance_units,
			tier
		)
		VALUES ($1, 'GH', 'GHS', $2, $3)
	`, teamID, balance, tier); err != nil {
		t.Fatalf("seed team wallet: %v", err)
	}
	return teamID
}

func seedEmailRate(
	t *testing.T,
	pool *pgxpool.Pool,
	tier string,
	costUnits int64,
) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO product_rates (
			product,
			meter,
			billing_market,
			tier,
			currency,
			cost_units,
			effective_from
		)
		VALUES (
			'email',
			'email_recipient',
			'GH',
			$1,
			'GHS',
			$2,
			now() - interval '1 day'
		)
	`, tier, costUnits); err != nil {
		t.Fatalf("seed email rate: %v", err)
	}
}

func seedAllowancePolicy(
	t *testing.T,
	pool *pgxpool.Pool,
	product string,
	meter string,
	tier string,
	quantity int64,
) uuid.UUID {
	t.Helper()
	var policyID uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO allowance_policies (
			product,
			meter,
			billing_market,
			tier,
			included_quantity,
			effective_from
		)
		VALUES (
			$1,
			$2,
			'GH',
			$3,
			$4,
			date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		)
		RETURNING id
	`, product, meter, tier, quantity).Scan(&policyID); err != nil {
		t.Fatalf("seed allowance policy: %v", err)
	}
	return policyID
}

func chargeEmail(
	t *testing.T,
	pool *pgxpool.Pool,
	input billing.EmailChargeInput,
) billing.Charge {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin billing transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	charge, err := billing.NewRepository(pool).ChargeEmail(ctx, tx, input)
	if err != nil {
		t.Fatalf("charge email: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit billing transaction: %v", err)
	}
	return charge
}
