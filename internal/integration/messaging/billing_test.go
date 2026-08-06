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
		t.Fatalf("expected no pre-granted allowances, got %d", count)
	}
	if count := requireCount(
		t,
		pool,
		`SELECT count(*) FROM team_wallets WHERE team_id = $1`,
		team.ID,
	); count != 1 {
		t.Fatalf("expected one team wallet, got %d", count)
	}
}

func TestAllowancePolicyGeneratesMonthlyAllowanceOnDemand(t *testing.T) {
	pool := openFreshDatabase(t)
	seedBillingMarket(t, pool)
	teamID := seedBillingTeam(t, pool, "growth", 1_000)
	seedEmailRate(t, pool, "growth", 5)

	ctx := context.Background()
	var policyID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO allowance_policies (
			product,
			meter,
			billing_market,
			tier,
			included_quantity,
			effective_from
		)
		VALUES (
			'email',
			'email_recipient',
			'GH',
			'growth',
			100,
			date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		)
		RETURNING id
	`).Scan(&policyID); err != nil {
		t.Fatalf("seed allowance policy: %v", err)
	}

	if count := requireCount(
		t,
		pool,
		`SELECT count(*) FROM usage_allowances WHERE team_id = $1`,
		teamID,
	); count != 0 {
		t.Fatalf("expected no allowance before first usage, got %d", count)
	}

	messageID := uuid.New()
	charge := chargeEmail(t, pool, billing.EmailChargeInput{
		TeamID:         teamID,
		MessageID:      messageID,
		RecipientCount: 120,
	})
	if charge.Outcome != billing.OutcomeApplied {
		t.Fatalf("expected applied outcome, got %q", charge.Outcome)
	}
	if charge.AmountUnits != 100 {
		t.Fatalf("expected 100 charged units, got %d", charge.AmountUnits)
	}
	if charge.RemainingBalance != 900 {
		t.Fatalf("expected 900 remaining balance, got %d", charge.RemainingBalance)
	}
	if !charge.CoveredByAllowance {
		t.Fatal("expected charge to use allowance")
	}
	if charge.RemainingAllowance != 0 {
		t.Fatalf("expected exhausted allowance, got %d", charge.RemainingAllowance)
	}

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
			AND period_end = period_start + interval '1 month'
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
		t.Fatalf("unexpected allowance context %s/%s", product, meter)
	}
	if market != "GH" || tier != "growth" {
		t.Fatalf("unexpected allowance market/tier %s/%s", market, tier)
	}
	if included != 100 || consumed != 100 {
		t.Fatalf("unexpected allowance quantities included=%d consumed=%d", included, consumed)
	}
	if !isUTCMonth {
		t.Fatal("expected a UTC calendar-month allowance")
	}

	retry := chargeEmail(t, pool, billing.EmailChargeInput{
		TeamID:         teamID,
		MessageID:      messageID,
		RecipientCount: 120,
	})
	if retry.Outcome != billing.OutcomeAlreadyApplied {
		t.Fatalf("expected already_applied on retry, got %q", retry.Outcome)
	}
	if retry.AmountUnits != 100 || retry.RemainingBalance != 900 {
		t.Fatalf(
			"retry changed charge state amount=%d balance=%d",
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

func TestSMSAllowancePolicyGeneratesAndSplitsCharge(t *testing.T) {
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
		VALUES ('GH', 'GH', 'local', 'growth', 'GHS', 10, now() - interval '1 day');

		INSERT INTO allowance_policies (
			product,
			meter,
			billing_market,
			tier,
			included_quantity,
			effective_from
		)
		VALUES (
			'sms',
			'sms_segment',
			'GH',
			'growth',
			2,
			date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		);
	`); err != nil {
		t.Fatalf("seed SMS billing configuration: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin SMS billing transaction: %v", err)
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
		t.Fatalf("commit SMS billing transaction: %v", err)
	}

	if row.Outcome != "applied" {
		t.Fatalf("expected applied outcome, got %q", row.Outcome)
	}
	if !row.CoveredByAllowance {
		t.Fatal("expected SMS charge to use allowance")
	}
	if row.AmountUnits != 30 || row.BalanceUnits != 970 {
		t.Fatalf(
			"unexpected SMS charge amount=%d balance=%d",
			row.AmountUnits,
			row.BalanceUnits,
		)
	}
	if row.RemainingAllowance != 0 {
		t.Fatalf("expected exhausted SMS allowance, got %d", row.RemainingAllowance)
	}
	if got := requireString(
		t,
		pool,
		`SELECT product || ':' || meter FROM usage_allowances WHERE team_id = $1`,
		teamID,
	); got != "sms:sms_segment" {
		t.Fatalf("unexpected SMS allowance context %q", got)
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
		t.Fatalf("expected applied outcome, got %q", charge.Outcome)
	}
	if charge.CoveredByAllowance {
		t.Fatal("expected fully billable usage without a policy")
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
		t.Fatalf("expected no generated allowance, got %d", count)
	}
}

func TestDuePendingTierAppliesBeforeAllowanceGeneration(t *testing.T) {
	pool := openFreshDatabase(t)
	seedBillingMarket(t, pool)
	teamID := seedBillingTeam(t, pool, "growth", 1_000)
	seedEmailRate(t, pool, "scale", 7)

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE team_wallets
		SET pending_tier = 'scale',
			pending_tier_effective_at =
				date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		WHERE team_id = $1
	`, teamID); err != nil {
		t.Fatalf("schedule due tier: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO allowance_policies (
			product,
			meter,
			billing_market,
			tier,
			included_quantity,
			effective_from
		)
		VALUES (
			'email',
			'email_recipient',
			'GH',
			'scale',
			50,
			date_trunc('month', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
		)
	`); err != nil {
		t.Fatalf("seed scale allowance policy: %v", err)
	}

	charge := chargeEmail(t, pool, billing.EmailChargeInput{
		TeamID:         teamID,
		MessageID:      uuid.New(),
		RecipientCount: 20,
	})
	if charge.Outcome != billing.OutcomeAllowanceApplied {
		t.Fatalf("expected allowance_applied outcome, got %q", charge.Outcome)
	}
	if charge.Tier != "scale" {
		t.Fatalf("expected scale tier, got %q", charge.Tier)
	}
	if charge.AmountUnits != 0 || charge.RemainingAllowance != 30 {
		t.Fatalf(
			"unexpected allowance charge amount=%d remaining=%d",
			charge.AmountUnits,
			charge.RemainingAllowance,
		)
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
		t.Fatalf("read activated wallet tier: %v", err)
	}
	if walletTier != "scale" || pendingTier != nil {
		t.Fatalf("unexpected wallet tier state tier=%q pending=%v", walletTier, pendingTier)
	}
	if got := requireString(
		t,
		pool,
		`SELECT tier FROM usage_allowances WHERE team_id = $1`,
		teamID,
	); got != "scale" {
		t.Fatalf("expected scale allowance snapshot, got %q", got)
	}
}

func seedBillingMarket(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO currencies (code, minor_unit, is_enabled)
		VALUES ('GHS', 2, true);

		INSERT INTO billing_markets (code, currency, is_enabled)
		VALUES ('GH', 'GHS', true);
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
	teamID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO teams (
			id,
			name,
			market_code,
			phone,
			address,
			status
		)
		VALUES ($1, 'Billing Test', 'GH', '+233200000002', 'Accra', 'active');

		INSERT INTO team_wallets (
			team_id,
			billing_market,
			currency,
			balance_units,
			tier
		)
		VALUES ($1, 'GH', 'GHS', $2, $3);
	`, teamID, balance, tier); err != nil {
		t.Fatalf("seed billing team: %v", err)
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
