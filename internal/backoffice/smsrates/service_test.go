package smsrates

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type testStore struct {
	created CreateInput
}

func (store *testStore) List(context.Context, int32, int32) ([]SMSRate, error) {
	return nil, nil
}

func (store *testStore) Get(context.Context, uuid.UUID) (SMSRate, error) {
	return SMSRate{}, nil
}

func (store *testStore) Create(_ context.Context, input CreateInput) (SMSRate, error) {
	store.created = input
	return SMSRate{BillingMarket: input.BillingMarket, DestinationCountry: input.DestinationCountry, RouteType: input.RouteType, Tier: input.Tier, Currency: input.Currency}, nil
}

func (store *testStore) Close(context.Context, uuid.UUID, time.Time) (SMSRate, error) {
	return SMSRate{}, nil
}

func TestCreateNormalizesSMSRateContext(t *testing.T) {
	t.Parallel()

	repository := &testStore{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	result, err := service.Create(context.Background(), CreateInput{
		BillingMarket:      " gh ",
		DestinationCountry: " ng ",
		RouteType:          " INTL ",
		Tier:               " GROWTH ",
		Currency:           " ghs ",
		CostUnits:          10,
		EffectiveFrom:      time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("create SMS rate: %v", err)
	}
	if result.BillingMarket != "GH" || result.DestinationCountry != "NG" || result.RouteType != "intl" || result.Tier != "growth" || result.Currency != "GHS" {
		t.Fatalf("unexpected normalized SMS rate: %#v", result)
	}
}
