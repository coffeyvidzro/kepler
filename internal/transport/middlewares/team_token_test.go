package middlewares

import (
	"testing"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

func TestTokenPermissionsValidateStoredValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		wantOK bool
	}{
		{name: "known permissions", values: []string{string(tenant.PermissionSMSRead), string(tenant.PermissionSMSSend)}, wantOK: true},
		{name: "unknown permission", values: []string{string(tenant.PermissionSMSRead), "root:all"}},
		{name: "known but privileged permission", values: []string{string(tenant.PermissionTeamDelete)}},
		{name: "blank permission", values: []string{" "}},
		{name: "empty permissions"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			permissions, ok := tokenPermissions(test.values)
			if ok != test.wantOK {
				t.Fatalf("tokenPermissions() ok = %t, want %t", ok, test.wantOK)
			}
			if ok && len(permissions) != len(test.values) {
				t.Fatalf("tokenPermissions() returned %d permissions, want %d", len(permissions), len(test.values))
			}
		})
	}
}
