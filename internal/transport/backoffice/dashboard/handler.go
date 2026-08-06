package dashboard

import (
	"context"

	"github.com/labstack/echo/v5"

	backofficedashboard "github.com/coffeyvidzro/dugble/server/internal/backoffice/dashboard"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
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

func (handler *Handler) Stats(c *echo.Context) error {
	stats, err := handler.service.Stats(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, stats)
}
