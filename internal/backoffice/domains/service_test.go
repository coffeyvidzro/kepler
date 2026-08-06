package domains

import (
	"context"
	"testing"
)

func TestValidatePageUsesDefaultLimit(t *testing.T) {
	t.Parallel()

	limit, offset, err := validatePage(0, 0)
	if err != nil {
		t.Fatalf("validate page: %v", err)
	}
	if limit != defaultPageLimit || offset != 0 {
		t.Fatalf("expected limit=%d offset=0, got limit=%d offset=%d", defaultPageLimit, limit, offset)
	}
}

func TestGetRejectsInvalidID(t *testing.T) {
	t.Parallel()

	service := NewService(&Repository{})
	if _, err := service.Get(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("expected invalid domain ID error")
	}
}
