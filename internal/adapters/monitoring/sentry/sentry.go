package sentry

import (
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/coffeyvidzro/dugble/server/internal/config"
)

// Init initializes Sentry error monitoring when a DSN is configured.
func Init(configuration config.SentryConfig, environment string) error {
	configuration.DSN = strings.TrimSpace(configuration.DSN)
	if configuration.DSN == "" {
		return nil
	}
	return sentry.Init(sentry.ClientOptions{
		Dsn:              configuration.DSN,
		Environment:      strings.TrimSpace(environment),
		Release:          strings.TrimSpace(configuration.Release),
		SampleRate:       configuration.ErrorSampleRate,
		Debug:            configuration.Debug,
		AttachStacktrace: true,
		EnableTracing:    false,
		SendDefaultPII:   false,
	})
}

func Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}
