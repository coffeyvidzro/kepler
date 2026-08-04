package wallet

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

func TestRegisterRoutesRequiresWalletPermissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		path       string
		permission tenant.Permission
	}{
		{name: "wallet", path: "/wallet", permission: tenant.PermissionWalletRead},
		{name: "ledger", path: "/wallet/ledger", permission: tenant.PermissionWalletLedgerRead},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			router := echo.New()
			RegisterRoutes(router, &Handler{}, func(permission tenant.Permission) echo.MiddlewareFunc {
				return func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c *echo.Context) error {
						c.Response().Header().Set("X-Required-Permission", string(permission))
						return c.NoContent(http.StatusNoContent)
					}
				}
			})
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
			}
			if got := response.Header().Get("X-Required-Permission"); got != string(test.permission) {
				t.Fatalf("permission = %q, want %q", got, test.permission)
			}
		})
	}
}
