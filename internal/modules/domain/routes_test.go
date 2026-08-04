package domain

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
)

func TestRegisterRoutesRequiresExpectedPermissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		method     string
		path       string
		permission tenant.Permission
	}{
		{method: http.MethodGet, path: "/domains", permission: tenant.PermissionSenderDomainsRead},
		{method: http.MethodPost, path: "/domains", permission: tenant.PermissionSenderDomainsCreate},
		{method: http.MethodGet, path: "/domains/domain-id", permission: tenant.PermissionSenderDomainsRead},
		{method: http.MethodPost, path: "/domains/domain-id/verify", permission: tenant.PermissionSenderDomainsCreate},
		{method: http.MethodDelete, path: "/domains/domain-id", permission: tenant.PermissionSenderDomainsDelete},
	}

	for _, test := range tests {
		router := echo.New()
		RegisterRoutes(router, &Handler{}, domainPermissionProbe)
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s %s status = %d, want %d", test.method, test.path, response.Code, http.StatusNoContent)
		}
		if got := response.Header().Get("X-Required-Permission"); got != string(test.permission) {
			t.Fatalf("%s %s permission = %q, want %q", test.method, test.path, got, test.permission)
		}
	}
}

func TestCreateReturnsAcceptedWhileEmailInfrastructureProvisions(t *testing.T) {
	teamID := uuid.New()
	provisioner := &provisioningStub{tenant: emailtenant.Tenant{
		ID: uuid.New(), TeamID: teamID, Region: "eu-north-1", Status: emailtenant.StatusProvisioning,
	}}
	service := NewService(nil, checkProvider{}, checkDNS(true), provisioner)
	handler := NewHandler(service)
	router := echo.New()
	router.POST("/domains", handler.Create, provisioningAccessMiddleware(teamID))

	for attempt := 1; attempt <= 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/domains", bytes.NewBufferString(`{
			"domain":"example.com",
			"region":"eu-north-1",
			"custom_return_path":"send"
		}`))
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		if response.Code != http.StatusAccepted {
			t.Fatalf("attempt %d status = %d, want %d; body=%s", attempt, response.Code, http.StatusAccepted, response.Body.String())
		}
		if got := response.Header().Get("Retry-After"); got != strconv.Itoa(emailInfrastructureRetryAfterSeconds) {
			t.Fatalf("attempt %d Retry-After = %q", attempt, got)
		}

		var envelope struct {
			Success bool                 `json:"success"`
			Data    ProvisioningResponse `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("attempt %d decode response: %v", attempt, err)
		}
		if !envelope.Success || envelope.Data.Status != "provisioning" || envelope.Data.RetryAfterSeconds != emailInfrastructureRetryAfterSeconds {
			t.Fatalf("attempt %d response = %+v", attempt, envelope)
		}
	}

	if provisioner.requests != 2 {
		t.Fatalf("provisioning requests = %d, want 2", provisioner.requests)
	}
}

func domainPermissionProbe(permission tenant.Permission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Response().Header().Set("X-Required-Permission", string(permission))
			return c.NoContent(http.StatusNoContent)
		}
	}
}

func provisioningAccessMiddleware(teamID uuid.UUID) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			access := tenant.AccessContext{
				Actor: tenant.Actor{Type: tenant.ActorTypeUser, UserID: uuid.New()},
				Scope: tenant.Scope{TeamID: teamID, Role: tenant.RoleOwner, Status: tenant.StatusActive},
			}
			ctx := tenant.ContextWithAccess(c.Request().Context(), access)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
