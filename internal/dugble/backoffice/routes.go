package backoffice

import (
	"errors"

	"github.com/labstack/echo/v5"

	backofficeallowancepolicieshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/allowancepolicies"
	backofficebillingmarketshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/billingmarkets"
	backofficecurrencieshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/currencies"
	backofficedashboardhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/dashboard"
	backofficedomainhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/domains"
	backofficehealthhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/health"
	backofficeproductrateshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/productrates"
	backofficesmsrateshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/smsrates"
	backofficeteamshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/teams"
	backofficeusershttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/users"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
)

type routeHandlers struct {
	health            *backofficehealthhttp.Handler
	dashboard         *backofficedashboardhttp.Handler
	users             *backofficeusershttp.Handler
	teams             *backofficeteamshttp.Handler
	domains           *backofficedomainhttp.Handler
	currencies        *backofficecurrencieshttp.Handler
	billingMarkets    *backofficebillingmarketshttp.Handler
	smsRates          *backofficesmsrateshttp.Handler
	productRates      *backofficeproductrateshttp.Handler
	allowancePolicies *backofficeallowancepolicieshttp.Handler
}

func newRouteRegistrar(
	handlers routeHandlers,
	backofficeAccess ...echo.MiddlewareFunc,
) httptransport.Registrar {
	return func(router *echo.Echo) error {
		if router == nil {
			return errors.New("backoffice router is required")
		}
		if handlers.health == nil {
			return errors.New("backoffice health handler is required")
		}
		backofficehealthhttp.RegisterRoutes(router, handlers.health)

		if len(backofficeAccess) == 0 {
			return nil
		}
		if handlers.dashboard == nil ||
			handlers.users == nil ||
			handlers.teams == nil ||
			handlers.domains == nil ||
			handlers.currencies == nil ||
			handlers.billingMarkets == nil ||
			handlers.smsRates == nil ||
			handlers.productRates == nil ||
			handlers.allowancePolicies == nil {
			return errors.New("backoffice administrative handlers are required")
		}

		backofficedashboardhttp.RegisterRoutes(router, handlers.dashboard, backofficeAccess...)
		backofficeusershttp.RegisterRoutes(router, handlers.users, backofficeAccess...)
		backofficeteamshttp.RegisterRoutes(router, handlers.teams, backofficeAccess...)
		backofficedomainhttp.RegisterRoutes(router, handlers.domains, backofficeAccess...)
		backofficecurrencieshttp.RegisterRoutes(router, handlers.currencies, backofficeAccess...)
		backofficebillingmarketshttp.RegisterRoutes(router, handlers.billingMarkets, backofficeAccess...)
		backofficesmsrateshttp.RegisterRoutes(router, handlers.smsRates, backofficeAccess...)
		backofficeproductrateshttp.RegisterRoutes(router, handlers.productRates, backofficeAccess...)
		backofficeallowancepolicieshttp.RegisterRoutes(router, handlers.allowancePolicies, backofficeAccess...)
		return nil
	}
}
