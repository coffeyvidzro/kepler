package backoffice

import (
	"context"
	"errors"
	"fmt"
	"time"

	newrelicmonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/newrelic"
	sentrymonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/sentry"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	backofficeallowancepolicies "github.com/coffeyvidzro/dugble/server/internal/backoffice/allowancepolicies"
	backofficebillingmarkets "github.com/coffeyvidzro/dugble/server/internal/backoffice/billingmarkets"
	backofficecurrencies "github.com/coffeyvidzro/dugble/server/internal/backoffice/currencies"
	backofficedomains "github.com/coffeyvidzro/dugble/server/internal/backoffice/domains"
	backofficeproductrates "github.com/coffeyvidzro/dugble/server/internal/backoffice/productrates"
	backofficesmsrates "github.com/coffeyvidzro/dugble/server/internal/backoffice/smsrates"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	backofficeallowancepolicieshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/allowancepolicies"
	backofficebillingmarketshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/billingmarkets"
	backofficecurrencieshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/currencies"
	backofficedomainhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/domains"
	backofficehealthhttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/health"
	backofficeproductrateshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/productrates"
	backofficesmsrateshttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice/smsrates"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
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

	domainsService, err := backofficedomains.NewService(backofficedomains.NewRepository(db))
	if err != nil {
		return fail(fmt.Errorf("create backoffice domains service: %w", err))
	}
	currenciesService, err := backofficecurrencies.NewService(backofficecurrencies.NewRepository(db))
	if err != nil {
		return fail(fmt.Errorf("create backoffice currencies service: %w", err))
	}
	billingMarketsService, err := backofficebillingmarkets.NewService(backofficebillingmarkets.NewRepository(db))
	if err != nil {
		return fail(fmt.Errorf("create backoffice billing markets service: %w", err))
	}
	smsRatesService, err := backofficesmsrates.NewService(backofficesmsrates.NewRepository(db))
	if err != nil {
		return fail(fmt.Errorf("create backoffice SMS rates service: %w", err))
	}
	productRatesService, err := backofficeproductrates.NewService(backofficeproductrates.NewRepository(db))
	if err != nil {
		return fail(fmt.Errorf("create backoffice product rates service: %w", err))
	}
	allowancePoliciesService, err := backofficeallowancepolicies.NewService(backofficeallowancepolicies.NewRepository(db))
	if err != nil {
		return fail(fmt.Errorf("create backoffice allowance policies service: %w", err))
	}

	handlers := routeHandlers{
		health:            backofficehealthhttp.NewHandler(db),
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
		newRouteRegistrar(handlers, nil),
	)
	if err != nil {
		return fail(fmt.Errorf("create backoffice HTTP router: %w", err))
	}

	application, err := NewApplication(
		newrelicmonitoring.WrapHTTP(newRelic, router),
		":"+cfg.HTTPPort,
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
