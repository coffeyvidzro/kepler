package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	platformevent "github.com/coffeyvidzro/dugble/server/internal/platform/event"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
)

// Runtime contains the application-level services shared by server handlers.
type Runtime struct {
	Events *platformevent.Emitter
}

// New builds the application-level server runtime.
func New(dependencies Dependencies) (*Runtime, error) {
	if err := dependencies.validate(); err != nil {
		return nil, err
	}
	return &Runtime{
		Events: platformevent.NewEmitter(
			platformwebhook.NewEventSink(dependencies.WebhookEmitter),
		),
	}, nil
}

// Application owns the HTTP handler and its lifecycle configuration.
type Application struct {
	handler http.Handler
	start   echo.StartConfig
}

// NewApplication creates an HTTP application with production-safe timeouts.
func NewApplication(handler http.Handler, address string) (*Application, error) {
	if handler == nil {
		return nil, errors.New("HTTP handler is required")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("HTTP server address is required")
	}

	return &Application{
		handler: handler,
		start: echo.StartConfig{
			Address:         address,
			HideBanner:      true,
			HidePort:        true,
			GracefulTimeout: 15 * time.Second,
			BeforeServeFunc: func(httpServer *http.Server) error {
				httpServer.ReadHeaderTimeout = 5 * time.Second
				httpServer.ReadTimeout = 15 * time.Second
				httpServer.WriteTimeout = 30 * time.Second
				httpServer.IdleTimeout = 60 * time.Second
				return nil
			},
			OnShutdownError: func(err error) {
				slog.Error("HTTP server graceful shutdown failed", "error", err)
			},
		},
	}, nil
}

// Run serves HTTP requests until ctx is cancelled.
func (application *Application) Run(ctx context.Context) error {
	if application == nil || application.handler == nil {
		return errors.New("HTTP application is not configured")
	}
	if ctx == nil {
		return errors.New("server context is required")
	}

	slog.Info("starting HTTP server", "address", application.start.Address)
	if err := application.start.Start(ctx, application.handler); err != nil {
		return fmt.Errorf("run HTTP server: %w", err)
	}
	slog.Info("HTTP server stopped")
	return nil
}
