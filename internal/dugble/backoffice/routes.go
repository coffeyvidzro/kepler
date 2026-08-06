package backoffice

import (
	"errors"

	"github.com/labstack/echo/v5"

	backofficedomainhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/domains"
	backofficehealthhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/health"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
)

type routeHandlers struct {
	health  *backofficehealthhttp.Handler
	domains *backofficedomainhttp.Handler
}

func newRouteRegistrar(
	handlers routeHandlers,
	backofficeAccess echo.MiddlewareFunc,
) httptransport.Registrar {
	return func(router *echo.Echo) error {
		if router == nil {
			return errors.New("backoffice router is required")
		}
		if handlers.health == nil {
			return errors.New("backoffice health handler is required")
		}
		backofficehealthhttp.RegisterRoutes(router, handlers.health)

		// Administrative data must not be exposed until the backoffice access
		// middleware is implemented. The domains module is wired now so adding
		// that boundary only requires supplying the middleware here.
		if backofficeAccess != nil {
			if handlers.domains == nil {
				return errors.New("backoffice domains handler is required")
			}
			backofficedomainhttp.RegisterRoutes(router, handlers.domains, backofficeAccess)
		}
		return nil
	}
}
