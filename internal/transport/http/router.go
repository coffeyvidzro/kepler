package httptransport

import (
	"fmt"

	"github.com/arcjet/arcjet-go"
	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"

	httpmiddleware "github.com/coffeyvidzro/dugble/server/internal/transport/http/middleware"
)

// Registrar adds one cohesive set of routes to an Echo router.
type Registrar func(*echo.Echo) error

// RouterConfig contains cross-cutting HTTP transport configuration.
type RouterConfig struct {
	Development bool
	CORSOrigins []string
	Arcjet      *arcjet.Client
	BodyLimit   int64
	Idempotency httpmiddleware.IdempotencyConfig
	Middleware  []echo.MiddlewareFunc
}

// NewRouter builds the shared HTTP stack and invokes each route registrar.
func NewRouter(config RouterConfig, registrars ...Registrar) (*echo.Echo, error) {
	router := echo.New()
	router.Use(echomiddleware.RequestID())
	router.Use(httpmiddleware.AuditRequestContext)
	router.Use(echomiddleware.RequestLogger())
	router.Use(echomiddleware.Recover())
	bodyLimit := config.BodyLimit
	if bodyLimit <= 0 {
		bodyLimit = DefaultMaxRequestBodyBytes
	}
	router.Use(echomiddleware.BodyLimit(bodyLimit))
	router.Use(httpmiddleware.CORS(config.CORSOrigins))
	router.Use(httpmiddleware.Security(config.Development))
	if config.Arcjet != nil {
		router.Use(httpmiddleware.Arcjet(config.Arcjet))
	}
	if config.Idempotency.Repository != nil {
		router.Use(httpmiddleware.Idempotency(config.Idempotency))
	}
	for _, middleware := range config.Middleware {
		if middleware != nil {
			router.Use(middleware)
		}
	}
	for index, register := range registrars {
		if register == nil {
			continue
		}
		if err := register(router); err != nil {
			return nil, fmt.Errorf("register HTTP routes %d: %w", index+1, err)
		}
	}
	return router, nil
}
