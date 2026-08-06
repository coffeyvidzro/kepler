package allowancepolicies

import (
	"context"
	"testing"
	"time"
)

func TestUTCMonthBoundary(t *testing.T) {
	t.Parallel()

	boundary := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	if !isUTCMonthBoundary(boundary) {
		t.Fatal("expected UTC month boundary")
	}
}

func TestCreateRejectsNonBoundaryEffectiveTime(t *testing.T) {
	t.Parallel()

	service := NewService(&Repository{})
	_, err := service.Create(context.Background(), CreateInput{
		Product:          "email",
		Meter:            "email_recipient",
		BillingMarket:    "GH",
		Tier:             "growth",
		IncludedQuantity: 1000,
		EffectiveFrom:    time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected UTC month boundary validation error")
	}
}
