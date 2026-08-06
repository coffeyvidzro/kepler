package smsrates

import (
	"context"
	"testing"
	"time"
)

func TestCreateRejectsInvalidRouteType(t *testing.T) {
	t.Parallel()

	service := NewService(&Repository{})
	_, err := service.Create(context.Background(), CreateInput{
		BillingMarket:      " gh ",
		DestinationCountry: " ng ",
		RouteType:          "premium",
		Tier:               "growth",
		Currency:           "GHS",
		CostUnits:          10,
		EffectiveFrom:      time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected invalid route type error")
	}
}
