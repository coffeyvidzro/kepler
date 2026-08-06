package backoffice

import (
	"context"
	"errors"
	"fmt"
	"time"

	newrelicmonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/newrelic"
	sentrymonitoring "github.com/coffeyvidzro/dugble/server/internal/adapters/monitoring/sentry"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	backofficeapp "github.com/coffeyvidzro/dugble/server/internal/backoffice"
	"github.com/coffeyvidzro/dugble/server/internal/config"
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

	cfg, err := config.LoadBackoffice()
	if err != nil {
		return fail(fmt.Errorf("load backoffice configuration: %w", err))
	}
	if err := sentrymonitoring.Init(cfg.Sentry, cfg.AppEnv); err != nil {
		return fail(fmt.Errorf("initialize Sentry: %w", err))
	}
	cleanups.Add(func() { sentrymonitoring.Flush(5 * time.Second) })

	newRelic, err := newrelicmonitoring.New(backofficeapp.ServiceName, cfg.AppEnv, cfg.NewRelic)
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

	service, err := backofficeapp.NewService(db)
	if err != nil {
		return fail(fmt.Errorf("create backoffice service: %w", err))
	}
	router, err := httptransport.NewRouter(
		httptransport.RouterConfig{
			Development: cfg.IsDevelopment(),
			CORSOrigins: cfg.CORSOrigins,
		},
		newRouteRegistrar(service),
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
