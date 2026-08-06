package backoffice

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
)

type HealthHandler struct {
	db *pgxpool.Pool
}

func NewHealthHandler(db *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Live(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "dugble-backoffice",
	})
}

func (h *HealthHandler) Ready(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	if h.db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"checks": map[string]string{"postgres": "unconfigured"},
		})
	}

	if err := h.db.Ping(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"checks": map[string]string{"postgres": "unavailable"},
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "ready",
		"checks": map[string]string{"postgres": "ok"},
	})
}
