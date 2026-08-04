package billing

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestObserveCommittedRecordsAuthorizationReceipt(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	teamID, messageID := uuid.New(), uuid.New()
	NewService(nil).ObserveCommitted(context.Background(), CommittedAuthorization{
		Authorization: Authorization{
			Outcome: OutcomeApplied, MarketCode: "GH", Currency: "GHS", Tier: "growth",
			Product: ProductSMS, UnitCostUnits: 6500, Quantity: 2,
			AmountUnits: 13000, RemainingBalance: 87000,
		},
		Channel: ChannelSMS, TeamID: teamID, MessageID: messageID,
	})

	logged := output.String()
	for _, expected := range []string{
		`"msg":"billing authorization committed"`,
		`"billing_channel":"sms"`,
		`"team_id":"` + teamID.String() + `"`,
		`"message_id":"` + messageID.String() + `"`,
		`"billing_outcome":"applied"`,
		`"billing_product":"sms"`,
		`"amount_units":13000`,
		`"remaining_balance_units":87000`,
	} {
		if !strings.Contains(logged, expected) {
			t.Fatalf("committed authorization log missing %s: %s", expected, logged)
		}
	}
}
