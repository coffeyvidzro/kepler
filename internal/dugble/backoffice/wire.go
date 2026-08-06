package backoffice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/labstack/echo/v5"

	newrelicmonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/newrelic"
	sentrymonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/sentry"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	backofficeallowancepolicies "github.com/coffeyvidzro/dugble/server/internal/backoffice/allowancepolicies"
	backofficebillingmarkets "github.com/coffeyvidzro/dugble/server/internal/backoffice/billingmarkets"
	backofficecurrencies "github.com/coffeyvidzro/dugble/server/internal/backoffice/currencies"
	backofficedashboard "github.com/coffeyvidzro/dugble/server/internal/backoffice/dashboard"
	backofficedomains "github.com/coffeyvidzro/dugble/server/internal/backoffice/domains"
	backofficeproductrates "github.com/coffeyvidzro/dugble/server/internal/backoffice/productrates"
	backofficesenderids "github.com/coffeyvidzro/dugble/server/internal/backoffice/senderids"
	backofficesms "github.com/coffeyvidzro/dugble/server/internal/backoffice/sms"
	backofficesmsrates "github.com/coffeyvidzro/dugble/server/internal/backoffice/smsrates"
	backofficeteams "github.com/coffeyvidzro/dugble/server/internal/backoffice/teams"
	backofficeusers "github.com/coffeyvidzro/dugble/server/internal/backoffice/users"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	authmodule "github.com/coffeyvidzro/dugble/server/internal/modules/auth"
	sessionmodule "github.com/coffeyvidzro/dugble/server/internal/modules/session"
	backofficehttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
	backofficeallowancepolicieshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/allowancepolicies"
	backofficebillingmarketshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/billingmarkets"
	backofficecurrencieshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/currencies"
	backofficedashboardhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/dashboard"
	backofficedomainhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/domains"
	backofficehealthhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/health"
	backofficeproductrateshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/productrates"
	backofficesenderidshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/senderids"
	backofficesmshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/sms"
	backofficesmsrateshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/smsrates"
	backofficeteamshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/teams"
	backofficeusershttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/users"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	httpmiddleware "github.com/coffeyvidzro/dugble/server/internal/transport/http/middleware"
)

// Wire builds the backoffice process and returns cleanup for all resources.
func Wire(ctx context.Context) (*Application, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("backoffice wiring context is required")
	}

	cleanups := &cleanupStack{}
	fail := func(err error) (*Application, func(), error) {
		cleanups.Run()
		return nil, nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(fmt.Errorf("load configuration: %w", err))
	}
	if err := sentrymonitoring.Init(cfg.Sentry, cfg.AppEnv); err != nil {
		return fail(fmt.Errorf("initialize Sentry: %w", err))
	}
	cleanups.Add(func() { sentrymonitoring.Flush(5 * time.Second) })

	newRelic, err := newrelicmonitoring.New(serviceName, cfg.AppEnv, cfg.NewRelic)
	if err != nil {
		return fail(fmt.Errorf("initialize New Relic: %w", err))
	}
	cleanups.Add(func() { newrelicmonitoring.Shutdown(newRelic, 5*time.Second) })

	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()

	db, err := postgres.New(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return fail(fmt.Errorf("initialize PostgreSQL: %w", err))
	}
	cleanups.Add(db.Close)

	sessionRepository := sessionmodule.NewRepository(db)
	authRepository := authmodule.NewRepository(db)
	authMiddleware := httpmiddleware.SessionAuth(httpmiddleware.SessionAuthConfig{
		Sessions: sessionRepository,
		Users:    authRepository,
	})
	adminMiddleware := backofficehttp.RequireAdmin(cfg.Backoffice.AdminEmails)
	csrfMiddleware := httpmiddleware.CSRF(httpmiddleware.CSRFConfig{
		Development:    cfg.IsDevelopment(),
		TrustedOrigins: cfg.CORSOrigins,
		TokenLookup:    "form:csrf,header:" + echo.HeaderXCSRFToken,
		CookieName:     "dugble_backoffice_csrf",
	})

	dashboardService := backofficedashboard.NewService(backofficedashboard.NewRepository(db))
	usersService := backofficeusers.NewService(backofficeusers.NewRepository(db))
	teamsService := backofficeteams.NewService(backofficeteams.NewRepository(db))
	smsService := backofficesms.NewService(backofficesms.NewRepository(db))
	senderIDsService := backofficesenderids.NewService(backofficesenderids.NewRepository(db))
	domainsService := backofficedomains.NewService(backofficedomains.NewRepository(db))
	currenciesService := backofficecurrencies.NewService(backofficecurrencies.NewRepository(db))
	billingMarketsService := backofficebillingmarkets.NewService(backofficebillingmarkets.NewRepository(db))
	smsRatesService := backofficesmsrates.NewService(backofficesmsrates.NewRepository(db))
	productRatesService := backofficeproductrates.NewService(backofficeproductrates.NewRepository(db))
	allowancePoliciesService := backofficeallowancepolicies.NewService(backofficeallowancepolicies.NewRepository(db))

	handlers := routeHandlers{
		health:            backofficehealthhttp.NewHandler(db),
		dashboard:         backofficedashboardhttp.NewHandler(dashboardService),
		users:             backofficeusershttp.NewHandler(usersService),
		teams:             backofficeteamshttp.NewHandler(teamsService),
		sms:               backofficesmshttp.NewHandler(smsService),
		senderIDs:         backofficesenderidshttp.NewHandler(senderIDsService),
		domains:           backofficedomainhttp.NewHandler(domainsService),
		currencies:        backofficecurrencieshttp.NewHandler(currenciesService),
		billingMarkets:    backofficebillingmarketshttp.NewHandler(billingMarketsService),
		smsRates:          backofficesmsrateshttp.NewHandler(smsRatesService),
		productRates:      backofficeproductrateshttp.NewHandler(productRatesService),
		allowancePolicies: backofficeallowancepolicieshttp.NewHandler(allowancePoliciesService),
	}
	router, err := httptransport.NewRouter(
		httptransport.RouterConfig{
			Development: cfg.IsDevelopment(),
			CORSOrigins: cfg.CORSOrigins,
		},
		backofficehttp.RegisterWeb,
		newRouteRegistrar(handlers, authMiddleware, adminMiddleware, csrfMiddleware),
	)
	if err != nil {
		return fail(fmt.Errorf("create backoffice HTTP router: %w", err))
	}

	application, err := NewApplication(
		newrelicmonitoring.WrapHTTP(newRelic, router),
		":"+cfg.Backoffice.HTTPPort,
	)
	if err != nil {
		return fail(fmt.Errorf("create backoffice HTTP application: %w", err))
	}
	return application, cleanups.Run, nil
}

type cleanupStack struct {
	functions []func()
}

func (stack *cleanupStack) Add(cleanup func()) {
	if stack == nil || cleanup == nil {
		return
	}
	stack.functions = append(stack.functions, cleanup)
}

func (stack *cleanupStack) Run() {
	if stack == nil {
		return
	}
	for index := len(stack.functions) - 1; index >= 0; index-- {
		stack.functions[index]()
	}
	stack.functions = nil
}
