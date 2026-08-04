// Package monitoring is retained as a compatibility bridge. New code should
// import internal/platform/monitoring.
package monitoring

import (
	"time"

	"github.com/coffeyvidzro/dugble/server/internal/config"
	platformmonitoring "github.com/coffeyvidzro/dugble/server/internal/platform/monitoring"
)

func InitSentry(configuration config.SentryConfig, environment string) error {
	return platformmonitoring.InitSentry(configuration, environment)
}

func FlushSentry(timeout time.Duration) bool {
	return platformmonitoring.FlushSentry(timeout)
}
