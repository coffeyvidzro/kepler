package health

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewHandler(db *pgxpool.Pool, redisClient *redis.Client) *Handler {
	return &Handler{
		db:    db,
		redis: redisClient,
	}
}

// Live reports whether the application process is running.
func (h *Handler) Live(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "dugble-server",
	})
}

// Ready reports whether the application dependencies are available.
func (h *Handler) Ready(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(
		c.Request().Context(),
		3*time.Second,
	)
	defer cancel()

	status := http.StatusOK

	checks := map[string]string{
		"postgres": "ok",
		"redis":    "ok",
	}

	if h.db == nil {
		checks["postgres"] = "unconfigured"
		status = http.StatusServiceUnavailable
	} else if err := h.db.Ping(ctx); err != nil {
		checks["postgres"] = "unavailable"
		status = http.StatusServiceUnavailable
	}

	if h.redis == nil {
		checks["redis"] = "unconfigured"
		status = http.StatusServiceUnavailable
	} else if err := h.redis.Ping(ctx).Err(); err != nil {
		checks["redis"] = "unavailable"
		status = http.StatusServiceUnavailable
	}

	readiness := "ready"

	if status != http.StatusOK {
		readiness = "not_ready"
	}

	return c.JSON(status, map[string]any{
		"status": readiness,
		"checks": checks,
	})
}
