package email

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestSendRequiresIdempotencyKey(t *testing.T) {
	router := echo.New()
	router.POST("/emails", NewHandler(nil).Send)
	request := httptest.NewRequest(http.MethodPost, "/emails", strings.NewReader(`{}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("POST /emails status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "Idempotency-Key") {
		t.Fatalf("POST /emails response does not explain idempotency requirement: %s", response.Body.String())
	}
}
