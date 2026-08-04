package tenant

import "testing"

func TestMembershipActiveRequiresActiveTeam(t *testing.T) {
	t.Parallel()

	membership := Membership{Status: StatusActive, TeamStatus: StatusDisabled}
	if membership.Active() {
		t.Fatalf("disabled team membership must not be active")
	}
}

func TestMembershipActiveAllowsLegacyEmptyTeamStatus(t *testing.T) {
	t.Parallel()

	membership := Membership{Status: StatusActive}
	if !membership.Active() {
		t.Fatalf("empty team status should keep existing active membership behavior")
	}
}
