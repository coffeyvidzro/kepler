package csrf_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	transportcsrf "github.com/coffeyvidzro/dugble/server/internal/transport/csrf"
	"github.com/coffeyvidzro/dugble/server/internal/transport/middlewares"
)

func TestTokenIssuesCSRFToken(t *testing.T) {
	t.Parallel()

	router := echo.New()
	handler := transportcsrf.NewHandler()
	router.GET(
		"/csrf",
		handler.Token,
		middlewares.CSRF(middlewares.CSRFConfig{Development: true}),
	)

	request := httptest.NewRequest(http.MethodGet, "/csrf", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Token string `json:"csrf_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatal("expected successful response")
	}
	if response.Data.Token == "" {
		t.Fatal("expected csrf token in response")
	}

	cookies := recorder.Result().Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "dugble_csrf" {
			if cookie.Value == "" {
				t.Fatal("expected csrf cookie value")
			}
			return
		}
	}

	t.Fatal("expected dugble_csrf cookie")
}
