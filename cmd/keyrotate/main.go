package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/database"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/platform/keyrotation"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cipher, err := authnz.NewSecretCipherKeyring(strings.Split(os.Getenv("ENCRYPTION_KEYS"), ","))
	if err != nil {
		return fmt.Errorf("load encryption keyring: %w", err)
	}
	db, err := database.NewPostgres(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer db.Close()
	result, err := keyrotation.New(db, cipher).Rotate(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("primary_key=%s scanned=%d rotated=%d\n", cipher.PrimaryKeyID(), result.Scanned, result.Rotated)
	return nil
}
