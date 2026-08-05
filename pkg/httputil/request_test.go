package httputil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

func TestReadBodyRestoresRequestBody(t *testing.T) {
	c := newTestContext(http.MethodPost, "/", `{"value":"test"}`)

	body, err := ReadBody(c, 1024)
	if err != nil {
		t.Fatalf("ReadBody() error = %v", err)
	}
	if got := string(body); got != `{"value":"test"}` {
		t.Fatalf("body = %q", got)
	}
	restored, err := io.ReadAll(c.Request().Body)
	if err != nil {
		t.Fatalf("read restored body: %v", err)
	}
	if got := string(restored); got != string(body) {
		t.Fatalf("restored body = %q, want %q", got, body)
	}
}

func TestReadBodyRejectsOversizedBody(t *testing.T) {
	c := newTestContext(http.MethodPost, "/", "1234")

	err := func() error {
		_, readErr := ReadBody(c, 3)
		return readErr
	}()
	if !apperrors.IsCode(err, apperrors.CodePayloadTooLarge) {
		t.Fatalf("ReadBody() error = %v, want payload too large", err)
	}
}

func TestDecodeJSONIsStrict(t *testing.T) {
	type request struct {
		Value string `json:"value"`
	}

	for name, body := range map[string]string{
		"unknown field":  `{"value":"test","extra":true}`,
		"trailing value": `{"value":"test"} {"value":"again"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestContext(http.MethodPost, "/", body)
			var decoded request
			if err := DecodeJSON(c, &decoded, 1024); err == nil {
				t.Fatal("DecodeJSON() error = nil")
			}
		})
	}
}

func TestQueryInt32(t *testing.T) {
	c := newTestContext(http.MethodGet, "/?valid=42&invalid=nope&overflow=2147483648", "")

	if got := QueryInt32(c, "valid"); got != 42 {
		t.Fatalf("valid = %d", got)
	}
	if got := QueryInt32(c, "invalid"); got != 0 {
		t.Fatalf("invalid = %d", got)
	}
	if got := QueryInt32(c, "overflow"); got != 0 {
		t.Fatalf("overflow = %d", got)
	}
}

func newTestContext(method, target, body string) *echo.Context {
	router := echo.New()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	response := httptest.NewRecorder()
	return router.NewContext(request, response)
}
