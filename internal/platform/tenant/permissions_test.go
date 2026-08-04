package tenant

import "testing"

func TestVerifyPermissionsByRole(t *testing.T) {
	privileged := []Permission{
		PermissionVerifyRead,
		PermissionVerifySend,
		PermissionVerifyCheck,
		PermissionVerifyManage,
	}
	for _, role := range []string{RoleOwner, RoleAdmin} {
		for _, permission := range privileged {
			if !Can(role, permission) {
				t.Fatalf("Can(%q, %q) = false, want true", role, permission)
			}
		}
	}

	if !Can(RoleMember, PermissionVerifyRead) {
		t.Fatalf("Can(%q, %q) = false, want true", RoleMember, PermissionVerifyRead)
	}
	memberWrites := []Permission{
		PermissionVerifySend,
		PermissionVerifyCheck,
		PermissionVerifyManage,
	}
	for _, permission := range memberWrites {
		if Can(RoleMember, permission) {
			t.Fatalf("Can(%q, %q) = true, want false", RoleMember, permission)
		}
	}
}
