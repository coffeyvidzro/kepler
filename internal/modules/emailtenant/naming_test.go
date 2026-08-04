package emailtenant

import (
	"testing"

	"github.com/google/uuid"
)

func TestAWSExternalNameIsStableAndOpaque(t *testing.T) {
	teamID := uuid.MustParse("80d3f812-8ae4-4e19-aef4-16d93fa64015")
	got := AWSExternalName(teamID)
	want := "dugble-t-80d3f8128ae44e19aef416d93fa64015"
	if got != want {
		t.Fatalf("AWSExternalName() = %q, want %q", got, want)
	}
	if len(got) > 64 {
		t.Fatalf("AWSExternalName() length = %d, want at most 64", len(got))
	}
}

func TestParseTeamIDRejectsInvalidValue(t *testing.T) {
	if _, err := ParseTeamID("not-a-uuid"); err == nil {
		t.Fatal("ParseTeamID() expected an error")
	}
}
