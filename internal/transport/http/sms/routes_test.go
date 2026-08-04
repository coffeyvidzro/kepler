package sms

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

func TestRegisterRoutesRequiresExpectedPermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		permission tenant.Permission
	}{
		{name: "list", method: http.MethodGet, path: "/sms", permission: tenant.PermissionSMSRead},
		{name: "send", method: http.MethodPost, path: "/sms", permission: tenant.PermissionSMSSend},
		{name: "batch send", method: http.MethodPost, path: "/sms/batch", permission: tenant.PermissionSMSSend},
		{name: "get", method: http.MethodGet, path: "/sms/03e91c4f-2b68-4380-a6d8-ce01211f1463", permission: tenant.PermissionSMSRead},
		{name: "sync status", method: http.MethodPost, path: "/sms/03e91c4f-2b68-4380-a6d8-ce01211f1463/sync-status", permission: tenant.PermissionSMSSend},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			router := echo.New()
			RegisterRoutes(router, &Handler{}, permissionProbe)

			request := httptest.NewRequest(tt.method, tt.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent {
				t.Fatalf("expected access middleware to stop request with status %d, got %d", http.StatusNoContent, response.Code)
			}
			if got := response.Header().Get("X-Required-Permission"); got != string(tt.permission) {
				t.Fatalf("expected permission %q, got %q", tt.permission, got)
			}
		})
	}
}

func permissionProbe(permission tenant.Permission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("X-Required-Permission", string(permission))
			return c.NoContent(http.StatusNoContent)
		}
	}
}
