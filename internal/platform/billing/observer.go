package billing

import (
	"context"
	"log/slog"
)

var (
	_ SMSBilling   = (*Service)(nil)
	_ EmailBilling = (*Service)(nil)
)

// ObserveCommitted records a durable billing decision after its enclosing
// transaction has committed. It deliberately has no error result: message
// acceptance must not be reversed by a best-effort observation failure.
func (s *Service) ObserveCommitted(ctx context.Context, committed CommittedAuthorization) {
	slog.InfoContext(ctx, "billing authorization committed",
		"billing_channel", committed.Channel,
		"team_id", committed.TeamID,
		"message_id", committed.MessageID,
		"billing_outcome", committed.Outcome,
		"billing_product", committed.Product,
		"market_code", committed.MarketCode,
		"currency", committed.Currency,
		"tier", committed.Tier,
		"unit_cost_units", committed.UnitCostUnits,
		"quantity", committed.Quantity,
		"amount_units", committed.AmountUnits,
		"remaining_balance_units", committed.RemainingBalance,
		"covered_by_allowance", committed.CoveredByAllowance,
		"remaining_allowance", committed.RemainingAllowance,
	)
}
