package allowancepolicies

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type testStore struct {
	created CreateInput
}

func (store *testStore) List(context.Context, int32, int32) ([]AllowancePolicy, error) {
	return nil, nil
}

func (store *testStore) Get(context.Context, uuid.UUID) (AllowancePolicy, error) {
	return AllowancePolicy{}, nil
}

func (store *testStore) Create(_ context.Context, input CreateInput) (AllowancePolicy, error) {
	store.created = input
	return AllowancePolicy{Product: input.Product, Meter: input.Meter, Cadence: input.Cadence, EffectiveFrom: input.EffectiveFrom}, nil
}

func (store *testStore) Close(context.Context, uuid.UUID, time.Time) (AllowancePolicy, error) {
	return AllowancePolicy{}, nil
}

func TestCreateDefaultsMonthlyCadence(t *testing.T) {
	t.Parallel()

	repository := &testStore{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	boundary := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	result, err := service.Create(context.Background(), CreateInput{
		Product:          " email ",
		Meter:            " email_recipient ",
		BillingMarket:    " gh ",
		Tier:             " growth ",
		IncludedQuantity: 1000,
		EffectiveFrom:    boundary,
	})
	if err != nil {
		t.Fatalf("create allowance policy: %v", err)
	}
	if result.Cadence != "monthly" || repository.created.Cadence != "monthly" {
		t.Fatalf("expected monthly cadence, got result=%q input=%q", result.Cadence, repository.created.Cadence)
	}
}

func TestCreateRejectsNonBoundaryEffectiveTime(t *testing.T) {
	t.Parallel()

	service, err := NewService(&testStore{})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = service.Create(context.Background(), CreateInput{
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
