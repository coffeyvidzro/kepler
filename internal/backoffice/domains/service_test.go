package domains

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type testStore struct {
	listLimit  int32
	listOffset int32
	getID      uuid.UUID
}

func (store *testStore) List(_ context.Context, limit int32, offset int32) ([]Domain, error) {
	store.listLimit = limit
	store.listOffset = offset
	return []Domain{{ID: uuid.NewString(), Name: "example.com"}}, nil
}

func (store *testStore) Get(_ context.Context, id uuid.UUID) (Domain, error) {
	store.getID = id
	return Domain{ID: id.String(), Name: "example.com"}, nil
}

func TestListUsesDefaultPageLimit(t *testing.T) {
	t.Parallel()

	repository := &testStore{}
	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	page, err := service.List(context.Background(), ListInput{})
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	if repository.listLimit != defaultPageLimit || page.Limit != defaultPageLimit {
		t.Fatalf("expected default limit %d, got repository=%d page=%d", defaultPageLimit, repository.listLimit, page.Limit)
	}
}

func TestGetRejectsInvalidID(t *testing.T) {
	t.Parallel()

	service, err := NewService(&testStore{})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, err := service.Get(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("expected invalid domain ID error")
	}
}
