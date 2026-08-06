package currencies

import (
	"context"
	"testing"
)

func TestNormalizeCode(t *testing.T) {
	t.Parallel()

	code, err := normalizeCode(" usd ")
	if err != nil {
		t.Fatalf("normalize currency code: %v", err)
	}
	if code != "USD" {
		t.Fatalf("expected USD, got %q", code)
	}
}

func TestCreateRejectsInvalidMinorUnit(t *testing.T) {
	t.Parallel()

	service := NewService(&Repository{})
	if _, err := service.Create(context.Background(), CreateInput{Code: "USD", MinorUnit: 7}); err == nil {
		t.Fatal("expected invalid minor unit error")
	}
}
