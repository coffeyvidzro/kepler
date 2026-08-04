package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/config"
	"github.com/coffeyvidzro/dugble/server/internal/database"
	"github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
)

func main() {
	if err := run(); err != nil {
		slog.Error("backoffice stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()

	db, err := database.NewPostgres(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("initialize PostgreSQL: %w", err)
	}
	defer db.Close()

	router, err := backoffice.NewRouter(
		cfg,
		backoffice.Dependencies{DB: db},
	)
	if err != nil {
		return fmt.Errorf("create backoffice router: %w", err)
	}

	server := echo.StartConfig{
		Address:         ":" + cfg.Backoffice.HTTPPort,
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
			slog.Error(
				"backoffice graceful shutdown failed",
				"error", err,
			)
		},
	}

	slog.Info("starting backoffice", "address", server.Address)
	if err := server.Start(ctx, router); err != nil {
		return fmt.Errorf("run backoffice: %w", err)
	}

	slog.Info("backoffice stopped")

	return nil
}
