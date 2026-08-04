package monitoring

import (
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/coffeyvidzro/dugble/server/internal/config"
)

// InitSentry initializes error monitoring when a DSN is configured.
// Performance tracing remains disabled because New Relic owns APM and distributed tracing.
func InitSentry(cfg config.SentryConfig, environment string) error {
	if cfg.DSN == "" {
		return nil
	}

	return sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      strings.TrimSpace(environment),
		Release:          cfg.Release,
		SampleRate:       cfg.ErrorSampleRate,
		Debug:            cfg.Debug,
		AttachStacktrace: true,
		EnableTracing:    false,
		SendDefaultPII:   false,
	})
}

// FlushSentry waits briefly for queued events to be delivered during shutdown.
func FlushSentry(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}
