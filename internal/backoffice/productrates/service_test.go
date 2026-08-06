package productrates

import (
	"context"
	"testing"
	"time"
)

func TestCreateRejectsWhitespaceIdentifiers(t *testing.T) {
	t.Parallel()

	service := NewService(&Repository{})
	_, err := service.Create(context.Background(), CreateInput{
		Product:       "email delivery",
		Meter:         "email_recipient",
		BillingMarket: "GH",
		Tier:          "growth",
		Currency:      "GHS",
		CostUnits:     1,
		EffectiveFrom: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected invalid product identifier error")
	}
}
