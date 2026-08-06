package billingmarkets

import (
	"context"
	"testing"
)

func TestNormalizeMarketCode(t *testing.T) {
	t.Parallel()

	code, err := normalizeMarketCode(" gh ")
	if err != nil {
		t.Fatalf("normalize billing market code: %v", err)
	}
	if code != "GH" {
		t.Fatalf("expected GH, got %q", code)
	}
}

func TestCreateRejectsInvalidCurrency(t *testing.T) {
	t.Parallel()

	service := NewService(&Repository{})
	if _, err := service.Create(context.Background(), CreateInput{Code: "GH", Currency: "cedis"}); err == nil {
		t.Fatal("expected invalid currency error")
	}
}
