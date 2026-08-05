package health

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v5"
	"github.com/redis/go-redis/v9"

	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewHandler(db *pgxpool.Pool, redisClient *redis.Client) *Handler {
	return &Handler{db: db, redis: redisClient}
}

func (handler *Handler) Live(c *echo.Context) error {
	return httputil.OK(c, map[string]string{"status": "ok", "service": "dugble-server"})
}

func (handler *Handler) Ready(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()
	status := http.StatusOK
	checks := map[string]string{"postgres": "ok", "redis": "ok"}
	if handler == nil || handler.db == nil {
		checks["postgres"] = "unconfigured"
		status = http.StatusServiceUnavailable
	} else if err := handler.db.Ping(ctx); err != nil {
		checks["postgres"] = "unavailable"
		status = http.StatusServiceUnavailable
	}
	if handler == nil || handler.redis == nil {
		checks["redis"] = "unconfigured"
		status = http.StatusServiceUnavailable
	} else if err := handler.redis.Ping(ctx).Err(); err != nil {
		checks["redis"] = "unavailable"
		status = http.StatusServiceUnavailable
	}
	readiness := "ready"
	if status != http.StatusOK {
		readiness = "not_ready"
	}
	return c.JSON(status, httputil.Response{
		Success: status == http.StatusOK,
		Data:    map[string]any{"status": readiness, "checks": checks},
	})
}
