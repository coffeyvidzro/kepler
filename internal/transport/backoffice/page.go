package backoffice

import (
	"net/http"

	"github.com/labstack/echo/v5"

	httpmiddleware "github.com/coffeyvidzro/dugble/server/internal/transport/http/middleware"
)

func RenderPage(c *echo.Context, templateName string, page PageData) error {
	csrf, _ := c.Get(httpmiddleware.CSRFContextKey).(string)
	page.CSRF = csrf

	return c.Render(http.StatusOK, templateName, page)
}
