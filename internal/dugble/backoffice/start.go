package backoffice

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// Start wires and runs backoffice until an interrupt or termination signal.
func Start() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, cleanup, err := Wire(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	return application.Run(ctx)
}
