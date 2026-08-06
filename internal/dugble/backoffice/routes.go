package backoffice

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	backofficeapp "github.com/coffeyvidzro/dugble/server/internal/backoffice"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type routeHandler struct {
	service *backofficeapp.Service
}

func newRouteRegistrar(service *backofficeapp.Service) httptransport.Registrar {
	return func(router *echo.Echo) error {
		if router == nil {
			return errors.New("backoffice router is required")
		}
		if service == nil {
			return errors.New("backoffice service is required")
		}

		handler := &routeHandler{service: service}
		router.GET("/health", handler.live)
		router.GET("/ready", handler.ready)
		return nil
	}
}

func (handler *routeHandler) live(c *echo.Context) error {
	return httputil.OK(c, map[string]string{
		"status":  "ok",
		"service": backofficeapp.ServiceName,
	})
}

func (handler *routeHandler) ready(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	status := http.StatusOK
	readiness := "ready"
	checks := map[string]string{"postgres": "ok"}
	if handler == nil || handler.service == nil {
		status = http.StatusServiceUnavailable
		readiness = "not_ready"
		checks["postgres"] = "unconfigured"
	} else if err := handler.service.Ready(ctx); err != nil {
		status = http.StatusServiceUnavailable
		readiness = "not_ready"
		checks["postgres"] = "unavailable"
	}

	return c.JSON(status, httputil.Response{
		Success: status == http.StatusOK,
		Data: map[string]any{
			"status": readiness,
			"checks": checks,
		},
	})
}
