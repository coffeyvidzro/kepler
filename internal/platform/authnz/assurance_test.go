package authnz

import (
	"testing"
	"time"
)

func TestAssuranceLevelMeets(t *testing.T) {
	t.Parallel()
	if !AssuranceLevelThree.Meets(AssuranceLevelTwo) {
		t.Fatal("aal3 should satisfy aal2")
	}
	if AssuranceLevelOne.Meets(AssuranceLevelTwo) {
		t.Fatal("aal1 must not satisfy aal2")
	}
	if AssuranceLevel("unknown").Meets(AssuranceLevelOne) {
		t.Fatal("unknown assurance must fail closed")
	}
	if AssuranceLevelThree.Meets(AssuranceLevel("unknown")) {
		t.Fatal("unknown required assurance must fail closed")
	}
}

func TestPrincipalRecentlyAuthenticated(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	principal := Principal{AssuranceLevel: AssuranceLevelTwo, AuthenticatedAt: now.Add(-time.Minute)}
	if !principal.RecentlyAuthenticated(AssuranceLevelTwo, 15*time.Minute, now) {
		t.Fatal("recent aal2 authentication should satisfy aal2")
	}
	if principal.RecentlyAuthenticated(AssuranceLevelTwo, 30*time.Second, now) {
		t.Fatal("stale authentication must require step-up")
	}
	principal.AuthenticatedAt = now.Add(time.Minute)
	if principal.RecentlyAuthenticated(AssuranceLevelTwo, 15*time.Minute, now) {
		t.Fatal("future authentication timestamps must fail closed")
	}
}
