package dashboard

import (
	"context"

	"github.com/labstack/echo/v5"

	backofficedashboard "github.com/coffeyvidzro/dugble/server/internal/backoffice/dashboard"
	backofficehttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
)

type service interface {
	Stats(context.Context) (backofficedashboard.Stats, error)
}

type Handler struct {
	service service
}

func NewHandler(service service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) Index(c *echo.Context) error {
	stats, err := handler.service.Stats(c.Request().Context())
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "dashboard.html", backofficehttp.PageData{
		Title: "Dashboard",
		Data:  stats,
	})
}
