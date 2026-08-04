package team

import (
	"testing"

	"github.com/google/uuid"
)

func TestTeamValidationNormalizesValidInput(t *testing.T) {
	id := uuid.New()
	if got, err := validateTeamName(" Example "); err != nil || got != "Example" {
		t.Fatalf("validateTeamName() = %q, %v", got, err)
	}
	if got, err := validateMarketCode(" gh "); err != nil || got != "GH" {
		t.Fatalf("validateMarketCode() = %q, %v", got, err)
	}
	if got, err := validateRequiredTeamField(" +233531184325 ", "Phone"); err != nil || got != "+233531184325" {
		t.Fatalf("validateRequiredTeamField() = %q, %v", got, err)
	}
	if got := normalizeOptionalTeamField(" vidzro.io "); got == nil || *got != "vidzro.io" {
		t.Fatalf("normalizeOptionalTeamField() = %v", got)
	}
	if got := normalizeOptionalTeamField(" "); got != nil {
		t.Fatalf("normalizeOptionalTeamField() = %v, want nil", got)
	}
	if got, err := validateTeamID(" " + id.String() + " "); err != nil || got != id {
		t.Fatalf("validateTeamID() = %s, %v", got, err)
	}
	if got, err := validateMemberRole(" admin "); err != nil || got != RoleAdmin {
		t.Fatalf("validateMemberRole() = %q, %v", got, err)
	}
	if got, err := normalizeInvitationEmail(" Person@Example.COM "); err != nil || got != "person@example.com" {
		t.Fatalf("normalizeInvitationEmail() = %q, %v", got, err)
	}
	if got, err := validateInvitationRole(""); err != nil || got != RoleMember {
		t.Fatalf("validateInvitationRole() = %q, %v", got, err)
	}
	if got, err := validateInvitationToken(" token "); err != nil || got != "token" {
		t.Fatalf("validateInvitationToken() = %q, %v", got, err)
	}
}

func TestTeamValidationRejectsInvalidInput(t *testing.T) {
	checks := []func() error{
		func() error { _, err := validateTeamName(" "); return err },
		func() error { _, err := validateMarketCode("NG"); return err },
		func() error { _, err := validateMarketCode("US"); return err },
		func() error { _, err := validateRequiredTeamField(" ", "Phone"); return err },
		func() error { _, err := validateTeamID("invalid"); return err },
		func() error { _, err := validateMemberID("invalid"); return err },
		func() error { _, err := validateMemberRole(RoleOwner); return err },
		func() error { _, err := normalizeInvitationEmail("invalid"); return err },
		func() error { _, err := validateInvitationToken(" "); return err },
	}
	for i, check := range checks {
		if err := check(); err == nil {
			t.Fatalf("validation check %d accepted invalid input", i)
		}
	}
}
