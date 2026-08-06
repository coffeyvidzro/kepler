package productrates

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type testStore struct{}

func (*testStore) List(context.Context, int32, int32) ([]ProductRate, error) {
	return nil, nil
}

func (*testStore) Get(context.Context, uuid.UUID) (ProductRate, error) {
	return ProductRate{}, nil
}

func (*testStore) Create(context.Context, CreateInput) (ProductRate, error) {
	return ProductRate{}, nil
}

func (*testStore) Close(context.Context, uuid.UUID, time.Time) (ProductRate, error) {
	return ProductRate{}, nil
}

func TestCreateRejectsWhitespaceIdentifiers(t *testing.T) {
	t.Parallel()

	service, err := NewService(&testStore{})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	_, err = service.Create(context.Background(), CreateInput{
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
