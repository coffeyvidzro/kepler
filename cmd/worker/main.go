package main

import (
	"log/slog"
	"os"

	dugbleworker "github.com/coffeyvidzro/dugble/server/internal/dugble/worker"
)

func main() {
	if err := dugbleworker.Start(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
