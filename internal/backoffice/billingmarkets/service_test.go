package billingmarkets

import (
	"context"
	"testing"
)

type testStore struct {
	created CreateInput
}

func (store *testStore) List(context.Context, int32, int32) ([]BillingMarket, error) {
	return nil, nil
}

func (store *testStore) Get(context.Context, string) (BillingMarket, error) {
	return BillingMarket{}, nil
}

func (store *testStore) Create(_ context.Context, input CreateInput) (BillingMarket, error) {
	store.created = input
	return BillingMarket{Code: input.Code, Currency: input.Currency}, nil
}

func (store *testStore) SetEnabled(context.Context, string, bool) (BillingMarket, error) {
	return BillingMarket{}, nil
}

func TestCreateNormalizesMarketAndCurrency(t *testing.T) {
	t.Parallel()

	repository := &testStore{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	result, err := service.Create(context.Background(), CreateInput{Code: " gh ", Currency: " ghs "})
	if err != nil {
		t.Fatalf("create billing market: %v", err)
	}
	if result.Code != "GH" || result.Currency != "GHS" {
		t.Fatalf("unexpected normalized market: %#v", result)
	}
}
