package senderid

import "testing"

func TestValidateCreateNormalizesSenderID(t *testing.T) {
	provider := "  internal  "
	name, countryCode, purpose, normalizedProvider, err := validateCreate(CreateRequest{
		Name:        "  Dugble  ",
		CountryCode: " gh ",
		Purpose:     " Transactional alerts ",
		Provider:    &provider,
	})
	if err != nil {
		t.Fatalf("validateCreate returned error: %v", err)
	}
	if name != "Dugble" {
		t.Fatalf("name = %q, want Dugble", name)
	}
	if countryCode != "GH" {
		t.Fatalf("countryCode = %q, want GH", countryCode)
	}
	if purpose != "Transactional alerts" {
		t.Fatalf("purpose = %q, want Transactional alerts", purpose)
	}
	if normalizedProvider == nil || *normalizedProvider != "internal" {
		t.Fatalf("provider = %v, want internal", normalizedProvider)
	}
}

func TestValidateCreateRejectsInvalidSenderID(t *testing.T) {
	_, _, _, _, err := validateCreate(CreateRequest{
		Name:        "sender-name-too-long",
		CountryCode: "US",
		Purpose:     "Transactional alerts",
	})
	if err == nil {
		t.Fatal("validateCreate returned nil error for long sender ID")
	}
}

func TestValidateCreateRejectsInvalidCountryCode(t *testing.T) {
	_, _, _, _, err := validateCreate(CreateRequest{
		Name:        "Dugble",
		CountryCode: "USA",
		Purpose:     "Transactional alerts",
	})
	if err == nil {
		t.Fatal("validateCreate returned nil error for invalid country code")
	}
}
