package senderid

import (
	"testing"

	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

func TestValidateCreateRoutesGhanaToMoolre(t *testing.T) {
	t.Parallel()

	name, country, _, provider, err := validateCreate(CreateRequest{
		Name:        "Dugble1",
		CountryCode: "gh",
		Purpose:     "Transactional notifications",
	})
	if err != nil {
		t.Fatalf("validateCreate() error = %v", err)
	}
	if name != "Dugble1" || country != "GH" {
		t.Fatalf("normalized name/country = %q/%q", name, country)
	}
	if provider == nil || *provider != platformsenderid.ProviderMoolre {
		t.Fatalf("provider = %v, want moolre", provider)
	}
}

func TestValidateCreateRejectsInvalidGhanaProvider(t *testing.T) {
	t.Parallel()

	provider := "other"
	_, _, _, _, err := validateCreate(CreateRequest{
		Name:        "Dugble1",
		CountryCode: "GH",
		Purpose:     "Transactional notifications",
		Provider:    &provider,
	})
	if err == nil {
		t.Fatal("validateCreate() error = nil")
	}
}

func TestValidateCreateRejectsInvalidNameBeforeDatabase(t *testing.T) {
	t.Parallel()

	_, _, _, _, err := validateCreate(CreateRequest{
		Name:        "12345",
		CountryCode: "GH",
		Purpose:     "Transactional notifications",
	})
	if err == nil {
		t.Fatal("validateCreate() error = nil")
	}
}
