package httptransport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
)

// Server owns the HTTP listener lifecycle and timeout policy.
type Server struct {
	handler http.Handler
	start   echo.StartConfig
}

func NewServer(handler http.Handler, address string) (*Server, error) {
	if handler == nil {
		return nil, errors.New("HTTP handler is required")
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, errors.New("HTTP server address is required")
	}
	return &Server{handler: handler, start: echo.StartConfig{
		Address: address, HideBanner: true, HidePort: true, GracefulTimeout: 15 * time.Second,
		BeforeServeFunc: func(server *http.Server) error {
			server.ReadHeaderTimeout = 5 * time.Second
			server.ReadTimeout = 15 * time.Second
			server.WriteTimeout = 30 * time.Second
			server.IdleTimeout = 60 * time.Second
			return nil
		},
		OnShutdownError: func(err error) { slog.Error("HTTP server graceful shutdown failed", "error", err) },
	}}, nil
}

func (server *Server) Run(ctx context.Context) error {
	if server == nil || server.handler == nil {
		return errors.New("HTTP server is not configured")
	}
	if ctx == nil {
		return errors.New("HTTP server context is required")
	}
	slog.Info("starting HTTP server", "address", server.start.Address)
	if err := server.start.Start(ctx, server.handler); err != nil {
		return fmt.Errorf("run HTTP server: %w", err)
	}
	slog.Info("HTTP server stopped")
	return nil
}
