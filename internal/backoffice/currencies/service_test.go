package currencies

import (
	"context"
	"testing"
)

type testStore struct {
	created CreateInput
}

func (store *testStore) List(context.Context, int32, int32) ([]Currency, error) {
	return nil, nil
}

func (store *testStore) Get(context.Context, string) (Currency, error) {
	return Currency{}, nil
}

func (store *testStore) Create(_ context.Context, input CreateInput) (Currency, error) {
	store.created = input
	return Currency{Code: input.Code, MinorUnit: input.MinorUnit}, nil
}

func (store *testStore) SetEnabled(context.Context, string, bool) (Currency, error) {
	return Currency{}, nil
}

func TestCreateNormalizesCurrencyCode(t *testing.T) {
	t.Parallel()

	repository := &testStore{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	result, err := service.Create(context.Background(), CreateInput{Code: " usd ", MinorUnit: 2})
	if err != nil {
		t.Fatalf("create currency: %v", err)
	}
	if result.Code != "USD" || repository.created.Code != "USD" {
		t.Fatalf("expected normalized USD code, got result=%q input=%q", result.Code, repository.created.Code)
	}
}

func TestCreateRejectsInvalidMinorUnit(t *testing.T) {
	t.Parallel()

	service, err := NewService(&testStore{})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, err := service.Create(context.Background(), CreateInput{Code: "USD", MinorUnit: 7}); err == nil {
		t.Fatal("expected invalid minor unit error")
	}
}
