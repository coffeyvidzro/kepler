package main

import (
	"log/slog"
	"os"

	dugblebackoffice "github.com/coffeyvidzro/dugble/server/internal/dugble/backoffice"
)

func main() {
	if err := dugblebackoffice.Start(); err != nil {
		slog.Error("backoffice stopped", "error", err)
		os.Exit(1)
	}
}
