package verify

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

func TestRegisterRoutesRequiresExpectedPermissions(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		permission tenant.Permission
	}{
		{name: "create service", method: http.MethodPost, path: "/verification-services", permission: tenant.PermissionVerifyManage},
		{name: "list services", method: http.MethodGet, path: "/verification-services", permission: tenant.PermissionVerifyRead},
		{name: "get service", method: http.MethodGet, path: "/verification-services/03e91c4f-2b68-4380-a6d8-ce01211f1463", permission: tenant.PermissionVerifyRead},
		{name: "update service", method: http.MethodPatch, path: "/verification-services/03e91c4f-2b68-4380-a6d8-ce01211f1463", permission: tenant.PermissionVerifyManage},
		{name: "create verification", method: http.MethodPost, path: "/verifications", permission: tenant.PermissionVerifySend},
		{name: "list verifications", method: http.MethodGet, path: "/verifications", permission: tenant.PermissionVerifyRead},
		{name: "get verification", method: http.MethodGet, path: "/verifications/03e91c4f-2b68-4380-a6d8-ce01211f1463", permission: tenant.PermissionVerifyRead},
		{name: "check", method: http.MethodPost, path: "/verifications/03e91c4f-2b68-4380-a6d8-ce01211f1463/check", permission: tenant.PermissionVerifyCheck},
		{name: "resend", method: http.MethodPost, path: "/verifications/03e91c4f-2b68-4380-a6d8-ce01211f1463/resend", permission: tenant.PermissionVerifySend},
		{name: "cancel", method: http.MethodPost, path: "/verifications/03e91c4f-2b68-4380-a6d8-ce01211f1463/cancel", permission: tenant.PermissionVerifySend},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := echo.New()
			RegisterRoutes(router, &Handler{}, verifyPermissionProbe)
			request := httptest.NewRequest(tt.method, tt.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
			if got := response.Header().Get("X-Required-Permission"); got != string(tt.permission) {
				t.Fatalf("permission = %q, want %q", got, tt.permission)
			}
		})
	}
}

func verifyPermissionProbe(permission tenant.Permission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("X-Required-Permission", string(permission))
			return c.NoContent(http.StatusNoContent)
		}
	}
}
