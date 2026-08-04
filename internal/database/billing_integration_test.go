package database_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/coffeyvidzro/dugble/server/internal/database/sqlc"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
)

func TestCreditTeamWalletIsIdempotentUnderConcurrency(t *testing.T) {
	db := billingTestDatabase(t)
	teamID := insertBillingTestTeam(t, db, 0)
	queries := dbsqlc.New(db)
	params := dbsqlc.CreditTeamWalletParams{
		TeamID: teamID, AmountUnits: 100, TransactionType: "deposit", ReferenceID: "concurrent-deposit",
	}

	successes, duplicates := runConcurrentMutations(t, 10, func(ctx context.Context, _ int) error {
		_, err := queries.CreditTeamWallet(ctx, params)
		return err
	})
	if successes != 1 || duplicates != 9 {
		t.Fatalf("credit results: successes=%d duplicates=%d", successes, duplicates)
	}
	assertWalletState(t, db, teamID, 100, 1, 100)
}

func TestAuthorizeSMSChargeUsesPartialAllowanceAndDebitsRemainder(t *testing.T) {
	db := billingTestDatabase(t)
	teamID := insertBillingTestTeam(t, db, 20000)
	insertSMSRate(t, db, 6500)
	insertAllowance(t, db, teamID, "sms_segment", 1)
	queries := dbsqlc.New(db)
	params := dbsqlc.AuthorizeSMSChargeParams{
		TeamID: teamID, ReferenceID: uuid.NewString(), DestinationCountry: "GH",
		Provider: "mnotify", RouteType: "standard", Quantity: 2,
	}

	first, err := queries.AuthorizeSMSCharge(context.Background(), params)
	if err != nil {
		t.Fatalf("authorize SMS: %v", err)
	}
	if first.Outcome != "applied" || first.Product != "sms" || first.CoveredByAllowance != true ||
		first.RemainingAllowance != 0 || first.AmountUnits != 6500 || first.BalanceUnits != 13500 {
		t.Fatalf("SMS authorization = %+v", first)
	}

	replay, err := queries.AuthorizeSMSCharge(context.Background(), params)
	if err != nil {
		t.Fatalf("replay SMS authorization: %v", err)
	}
	if replay.Outcome != "already_applied" || replay.AmountUnits != 6500 || replay.BalanceUnits != 13500 {
		t.Fatalf("replayed SMS authorization = %+v", replay)
	}
	assertUsageState(t, db, teamID, "sms_segment", 13500, 1, 1, 1)
}

func TestAuthorizeSMSChargeRejectsInsufficientBalanceWithoutConsumingAllowance(t *testing.T) {
	db := billingTestDatabase(t)
	teamID := insertBillingTestTeam(t, db, 1000)
	insertSMSRate(t, db, 6500)
	insertAllowance(t, db, teamID, "sms_segment", 1)
	result, err := dbsqlc.New(db).AuthorizeSMSCharge(context.Background(), dbsqlc.AuthorizeSMSChargeParams{
		TeamID: teamID, ReferenceID: uuid.NewString(), DestinationCountry: "GH",
		Provider: "mnotify", RouteType: "standard", Quantity: 2,
	})
	if err != nil {
		t.Fatalf("authorize insufficient SMS: %v", err)
	}
	if result.Outcome != "insufficient_balance" || result.BalanceUnits != 1000 {
		t.Fatalf("insufficient SMS authorization = %+v", result)
	}
	assertUsageState(t, db, teamID, "sms_segment", 1000, 0, 0, 0)
}

func TestAuthorizeSMSIsIdempotentUnderConcurrency(t *testing.T) {
	db := billingTestDatabase(t)
	teamID := insertBillingTestTeam(t, db, 6500)
	insertSMSRate(t, db, 6500)
	service := platformbilling.NewService(platformbilling.NewRepository(db))
	messageID := uuid.New()
	const workers = 10

	start := make(chan struct{})
	results := make(chan platformbilling.Outcome, workers)
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			tx, err := db.Begin(ctx)
			if err != nil {
				errorsChannel <- err
				return
			}
			defer func() { _ = tx.Rollback(ctx) }()
			result, err := service.AuthorizeSMS(ctx, tx, platformbilling.SMSAuthorizationInput{
				TeamID: teamID, MessageID: messageID, DestinationNumber: "+233241234567", Segments: 1,
			})
			if err == nil {
				err = tx.Commit(ctx)
			}
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- result.Outcome
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatalf("concurrent SMS authorization: %v", err)
	}
	applied, replayed := 0, 0
	for outcome := range results {
		switch outcome {
		case platformbilling.OutcomeApplied:
			applied++
		case platformbilling.OutcomeAlreadyApplied:
			replayed++
		default:
			t.Fatalf("concurrent SMS outcome = %q", outcome)
		}
	}
	if applied != 1 || replayed != workers-1 {
		t.Fatalf("concurrent outcomes: applied=%d replayed=%d", applied, replayed)
	}
	assertWalletState(t, db, teamID, 0, 1, -6500)
}

func TestAuthorizeEmailChargeUsesAllowanceThenPaidRate(t *testing.T) {
	db := billingTestDatabase(t)
	teamID := insertBillingTestTeam(t, db, 1000)
	insertProductRate(t, db, "email", "email_recipient", 936)
	insertAllowance(t, db, teamID, "email_recipient", 1)
	queries := dbsqlc.New(db)

	allowance, err := queries.AuthorizeEmailCharge(context.Background(), dbsqlc.AuthorizeEmailChargeParams{
		TeamID: teamID, ReferenceID: uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("authorize allowance email: %v", err)
	}
	if allowance.Outcome != "allowance_applied" || !allowance.CoveredByAllowance ||
		allowance.RemainingAllowance != 0 || allowance.AmountUnits != 0 || allowance.BalanceUnits != 1000 {
		t.Fatalf("allowance email authorization = %+v", allowance)
	}

	paidParams := dbsqlc.AuthorizeEmailChargeParams{TeamID: teamID, ReferenceID: uuid.NewString()}
	paid, err := queries.AuthorizeEmailCharge(context.Background(), paidParams)
	if err != nil {
		t.Fatalf("authorize paid email: %v", err)
	}
	if paid.Outcome != "applied" || paid.CoveredByAllowance || paid.AmountUnits != 936 || paid.BalanceUnits != 64 {
		t.Fatalf("paid email authorization = %+v", paid)
	}
	replay, err := queries.AuthorizeEmailCharge(context.Background(), paidParams)
	if err != nil {
		t.Fatalf("replay paid email: %v", err)
	}
	if replay.Outcome != "already_applied" || replay.BalanceUnits != 64 {
		t.Fatalf("replayed paid email = %+v", replay)
	}
	assertUsageState(t, db, teamID, "email_recipient", 64, 2, 1, 1)
}

func runConcurrentMutations(
	t *testing.T,
	count int,
	mutation func(context.Context, int) error,
) (successes int, noRows int) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan error, count)
	var workers sync.WaitGroup
	for index := range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			results <- mutation(ctx, index)
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, pgx.ErrNoRows):
			noRows++
		default:
			t.Fatalf("concurrent mutation: %v", err)
		}
	}
	return successes, noRows
}

func billingTestDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("INTEGRATION_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("INTEGRATION_DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect billing test database: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.Ping(ctx); err != nil {
		t.Fatalf("ping billing test database: %v", err)
	}
	return db
}

func insertBillingTestTeam(t *testing.T, db *pgxpool.Pool, balance int64) uuid.UUID {
	t.Helper()
	teamID := uuid.New()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO teams (id, name, market_code)
		VALUES ($1, 'Billing Integration', 'GH')
	`, teamID); err != nil {
		t.Fatalf("insert billing test team: %v", err)
	}
	if _, err := db.Exec(context.Background(), `
		INSERT INTO team_wallets (team_id, billing_market, currency, balance_units)
		VALUES ($1, 'GH', 'GHS', $2)
	`, teamID, balance); err != nil {
		t.Fatalf("insert billing test wallet: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM wallet_ledger WHERE team_id = $1`, teamID)
		_, _ = db.Exec(context.Background(), `DELETE FROM usage_authorizations WHERE team_id = $1`, teamID)
		_, _ = db.Exec(context.Background(), `DELETE FROM usage_allowances WHERE team_id = $1`, teamID)
		_, _ = db.Exec(context.Background(), `DELETE FROM teams WHERE id = $1`, teamID)
	})
	return teamID
}

func insertAllowance(t *testing.T, db *pgxpool.Pool, teamID uuid.UUID, meter string, quantity int64) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `
		INSERT INTO usage_allowances (
			team_id, meter, period_start, period_end, included_quantity
		) VALUES ($1, $2, now() - interval '1 day', now() + interval '1 day', $3)
	`, teamID, meter, quantity); err != nil {
		t.Fatalf("insert usage allowance: %v", err)
	}
}

func insertSMSRate(t *testing.T, db *pgxpool.Pool, cost int64) {
	t.Helper()
	_, _ = db.Exec(context.Background(), `
		DELETE FROM sms_rates
		WHERE billing_market = 'GH' AND destination_country = 'GH'
		  AND provider = 'mnotify' AND route_type = 'standard' AND tier = 'growth'
	`)
	if _, err := db.Exec(context.Background(), `
		INSERT INTO sms_rates (
			billing_market, destination_country, provider, route_type,
			tier, currency, cost_units, effective_from
		) VALUES ('GH', 'GH', 'mnotify', 'standard', 'growth', 'GHS', $1, '2020-01-01')
	`, cost); err != nil {
		t.Fatalf("insert SMS rate: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `
			DELETE FROM sms_rates
			WHERE billing_market = 'GH' AND destination_country = 'GH'
			  AND provider = 'mnotify' AND route_type = 'standard' AND tier = 'growth'
		`)
	})
}

func insertProductRate(t *testing.T, db *pgxpool.Pool, product, meter string, cost int64) {
	t.Helper()
	_, _ = db.Exec(context.Background(), `
		DELETE FROM product_rates
		WHERE product = $1 AND meter = $2 AND billing_market = 'GH' AND tier = 'growth'
	`, product, meter)
	if _, err := db.Exec(context.Background(), `
		INSERT INTO product_rates (
			product, meter, billing_market, tier, currency, cost_units, effective_from
		) VALUES ($1, $2, 'GH', 'growth', 'GHS', $3, '2020-01-01')
	`, product, meter, cost); err != nil {
		t.Fatalf("insert product rate: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `
			DELETE FROM product_rates
			WHERE product = $1 AND meter = $2 AND billing_market = 'GH' AND tier = 'growth'
		`, product, meter)
	})
}

func assertWalletState(
	t *testing.T,
	db *pgxpool.Pool,
	teamID uuid.UUID,
	wantBalance int64,
	wantLedgerCount int,
	wantLedgerTotal int64,
) {
	t.Helper()
	var balance, ledgerTotal int64
	var ledgerCount int
	if err := db.QueryRow(context.Background(), `
		SELECT wallet.balance_units, count(ledger.id), COALESCE(sum(ledger.amount_units), 0)
		FROM team_wallets AS wallet
		LEFT JOIN wallet_ledger AS ledger ON ledger.team_id = wallet.team_id
		WHERE wallet.team_id = $1
		GROUP BY wallet.balance_units
	`, teamID).Scan(&balance, &ledgerCount, &ledgerTotal); err != nil {
		t.Fatalf("read wallet state: %v", err)
	}
	if balance != wantBalance || ledgerCount != wantLedgerCount || ledgerTotal != wantLedgerTotal {
		t.Fatalf("wallet state: balance=%d ledger_count=%d ledger_total=%d; want %d, %d, %d",
			balance, ledgerCount, ledgerTotal, wantBalance, wantLedgerCount, wantLedgerTotal)
	}
}

func assertUsageState(
	t *testing.T,
	db *pgxpool.Pool,
	teamID uuid.UUID,
	meter string,
	wantBalance int64,
	wantAuthorizationCount int,
	wantLedgerCount int,
	wantConsumed int64,
) {
	t.Helper()
	var balance int64
	var authorizationCount, ledgerCount int
	var consumed int64
	if err := db.QueryRow(context.Background(), `
		SELECT
			wallet.balance_units,
			(SELECT count(*) FROM usage_authorizations WHERE team_id = wallet.team_id AND meter = $2),
			(SELECT count(*) FROM wallet_ledger WHERE team_id = wallet.team_id),
			COALESCE((SELECT sum(consumed_quantity) FROM usage_allowances WHERE team_id = wallet.team_id AND meter = $2), 0)
		FROM team_wallets AS wallet
		WHERE wallet.team_id = $1
	`, teamID, meter).Scan(&balance, &authorizationCount, &ledgerCount, &consumed); err != nil {
		t.Fatalf("read usage state: %v", err)
	}
	if balance != wantBalance || authorizationCount != wantAuthorizationCount ||
		ledgerCount != wantLedgerCount || consumed != wantConsumed {
		t.Fatalf("usage state: balance=%d authorizations=%d ledger=%d consumed=%d; want %d, %d, %d, %d",
			balance, authorizationCount, ledgerCount, consumed,
			wantBalance, wantAuthorizationCount, wantLedgerCount, wantConsumed)
	}
}
