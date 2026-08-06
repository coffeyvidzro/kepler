package backoffice

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestBackofficeAccessMiddleware(t *testing.T) {
	middleware := newBackofficeAccessMiddleware("secret-token")
	if middleware == nil {
		t.Fatal("expected middleware when token is configured")
	}

	tests := []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", authorization: "Basic secret-token", wantStatus: http.StatusUnauthorized},
		{name: "wrong token", authorization: "Bearer nope", wantStatus: http.StatusUnauthorized},
		{name: "valid token", authorization: "Bearer secret-token", wantStatus: http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.authorization != "" {
				req.Header.Set(echo.HeaderAuthorization, tt.authorization)
			}
			rec := httptest.NewRecorder()
			ctx := router.NewContext(req, rec)

			handler := middleware(func(c *echo.Context) error {
				return c.NoContent(http.StatusNoContent)
			})
			if err := handler(ctx); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestBackofficeAccessMiddlewareDisabledWithoutToken(t *testing.T) {
	if middleware := newBackofficeAccessMiddleware("  "); middleware != nil {
		t.Fatal("expected nil middleware when token is blank")
	}
}
