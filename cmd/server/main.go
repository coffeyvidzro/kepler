package main

import (
	"log/slog"
	"os"

	dugbleserver "github.com/coffeyvidzro/dugble/server/internal/dugble/server"
)

func main() {
	if err := dugbleserver.Start(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
