package user

import (
	"reflect"
	"testing"
)

func TestUserInputValidation(t *testing.T) {
	if got, err := validateID(" user-id "); err != nil || got != "user-id" {
		t.Fatalf("validateID() = %q, %v", got, err)
	}
	if got, err := validateName(" Person "); err != nil || got != "Person" {
		t.Fatalf("validateName() = %q, %v", got, err)
	}
	if got, err := validateEmail(" Person@Example.COM "); err != nil || got != "person@example.com" {
		t.Fatalf("validateEmail() = %q, %v", got, err)
	}
	if got, err := validatePassword(" 123456789012 "); err != nil || got != "123456789012" {
		t.Fatalf("validatePassword() = %q, %v", got, err)
	}
}

func TestUserInputValidationRejectsInvalidValues(t *testing.T) {
	for name, validate := range map[string]func() error{
		"id":       func() error { _, err := validateID(" "); return err },
		"name":     func() error { _, err := validateName(" "); return err },
		"email":    func() error { _, err := validateEmail("invalid"); return err },
		"password": func() error { _, err := validatePassword("short"); return err },
	} {
		if err := validate(); err == nil {
			t.Fatalf("%s validation accepted invalid input", name)
		}
	}
}

func TestUniqueEmailsNormalizesAndDeduplicates(t *testing.T) {
	got := uniqueEmails("Old@Example.com", " old@example.com ", "new@example.com", "")
	want := []string{"old@example.com", "new@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("uniqueEmails() = %#v, want %#v", got, want)
	}
}
