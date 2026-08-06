package health

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

const serviceName = "dugble-backoffice"

type Database interface {
	Ping(context.Context) error
}

type Handler struct {
	database Database
}

func NewHandler(database Database) *Handler {
	return &Handler{database: database}
}

func (handler *Handler) Live(c *echo.Context) error {
	return httputil.OK(c, map[string]string{
		"status":  "ok",
		"service": serviceName,
	})
}

func (handler *Handler) Ready(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	status := http.StatusOK
	readiness := "ready"
	checks := map[string]string{"postgres": "ok"}
	if handler == nil || handler.database == nil {
		status = http.StatusServiceUnavailable
		readiness = "not_ready"
		checks["postgres"] = "unconfigured"
	} else if err := handler.database.Ping(ctx); err != nil {
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
