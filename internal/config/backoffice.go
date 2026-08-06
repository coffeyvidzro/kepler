package config

import (
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// BackofficeConfig contains only the dependencies required by the
// independently deployed backoffice process.
type BackofficeConfig struct {
	AppEnv      string         `env:"APP_ENV" envDefault:"development"`
	HTTPPort    string         `env:"BACKOFFICE_HTTP_PORT" envDefault:"8081"`
	DatabaseURL string         `env:"DATABASE_URL,required,notEmpty"`
	CORSOrigins []string       `env:"BACKOFFICE_CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3001,http://127.0.0.1:3001"`
	NewRelic    NewRelicConfig `envPrefix:"NEW_RELIC_"`
	Sentry      SentryConfig   `envPrefix:"SENTRY_"`
}

func LoadBackoffice() (*BackofficeConfig, error) {
	_ = godotenv.Load()
	cfg := &BackofficeConfig{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	cfg.normalize()
	return cfg, nil
}

func (c *BackofficeConfig) IsDevelopment() bool {
	return c != nil && strings.EqualFold(c.AppEnv, "development")
}

func (c *BackofficeConfig) normalize() {
	if c == nil {
		return
	}
	c.AppEnv = strings.TrimSpace(c.AppEnv)
	c.HTTPPort = strings.TrimSpace(c.HTTPPort)
	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.CORSOrigins = normalizeStrings(c.CORSOrigins)
	c.NewRelic.LicenseKey = strings.TrimSpace(c.NewRelic.LicenseKey)
	c.Sentry.DSN = strings.TrimSpace(c.Sentry.DSN)
	c.Sentry.Release = strings.TrimSpace(c.Sentry.Release)
}
