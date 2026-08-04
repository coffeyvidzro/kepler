package config

import (
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type ProviderConfig struct {
	BaseURL string `env:"BASE_URL"`
	APIKey  string `env:"API_KEY"`
}

type CelcomConfig struct {
	BaseURL   string `env:"BASE_URL"`
	APIKey    string `env:"API_KEY"`
	PartnerID string `env:"PARTNER_ID"`
}

type VerifyConfig struct {
	HMACSecret string `env:"HMAC_SECRET"`
}

type AWSConfig struct {
	FromEmail                        string `env:"FROM_EMAIL,required,notEmpty"`
	Region                           string `env:"REGION,required,notEmpty"`
	AccessKey                        string `env:"ACCESS_KEY_ID"`
	SecretKey                        string `env:"SECRET_ACCESS_KEY"`
	SESConfigurationSet              string
	SESTransactionalConfigurationSet string
	SESMarketingConfigurationSet     string
	SESTenantName                    string
	SNSTopicARNs                     []string `env:"SNS_TOPIC_ARNS" envSeparator:","`
}

type BackofficeConfig struct {
	HTTPPort    string   `env:"HTTP_PORT" envDefault:"8081"`
	AdminEmails []string `env:"ADMIN_EMAILS" envSeparator:","`
}

type NewRelicConfig struct {
	LicenseKey                string `env:"LICENSE_KEY"`
	DistributedTracingEnabled bool   `env:"DISTRIBUTED_TRACING_ENABLED" envDefault:"true"`
	LogEnabled                bool   `env:"LOG_ENABLED" envDefault:"true"`
}

type SentryConfig struct {
	DSN             string  `env:"DSN"`
	Release         string  `env:"RELEASE"`
	ErrorSampleRate float64 `env:"ERROR_SAMPLE_RATE" envDefault:"1"`
	Debug           bool    `env:"DEBUG" envDefault:"false"`
}

type Config struct {
	AppEnv         string           `env:"APP_ENV"   envDefault:"development"`
	HTTPPort       string           `env:"HTTP_PORT" envDefault:"8080"`
	DatabaseURL    string           `env:"DATABASE_URL,required,notEmpty"`
	RedisURL       string           `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`
	CORSOrigins    []string         `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000,http://127.0.0.1:3000"`
	ArcjetKey      string           `env:"ARCJET_KEY,required,notEmpty"`
	FrontendURL    string           `env:"FRONTEND_URL" envDefault:"http://localhost:3000"`
	BackendURL     string           `env:"BACKEND_URL"  envDefault:"http://localhost:8080"`
	CookieDomain   string           `env:"COOKIE_DOMAIN"`
	EncryptionKeys []string         `env:"ENCRYPTION_KEYS" envSeparator:","`
	Verify         VerifyConfig     `envPrefix:"VERIFY_"`
	AWS            AWSConfig        `envPrefix:"AWS_"`
	NATSURL        string           `env:"NATS_URL" envDefault:"nats://localhost:4222"`
	Arkesel        ProviderConfig   `envPrefix:"ARKESEL_"`
	MNotify        ProviderConfig   `envPrefix:"MNOTIFY_"`
	Celcom         CelcomConfig     `envPrefix:"CELCOM_"`
	Backoffice     BackofficeConfig `envPrefix:"BACKOFFICE_"`
	NewRelic       NewRelicConfig   `envPrefix:"NEW_RELIC_"`
	Sentry         SentryConfig     `envPrefix:"SENTRY_"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) IsDevelopment() bool { return strings.EqualFold(c.AppEnv, "development") }

func (c *Config) normalize() {
	c.AppEnv = strings.TrimSpace(c.AppEnv)
	c.HTTPPort = strings.TrimSpace(c.HTTPPort)
	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.RedisURL = strings.TrimSpace(c.RedisURL)
	c.ArcjetKey = strings.TrimSpace(c.ArcjetKey)
	c.FrontendURL = strings.TrimRight(strings.TrimSpace(c.FrontendURL), "/")
	c.BackendURL = strings.TrimRight(strings.TrimSpace(c.BackendURL), "/")
	c.CookieDomain = strings.TrimSpace(c.CookieDomain)
	c.EncryptionKeys = normalizeStrings(c.EncryptionKeys)
	c.Verify.HMACSecret = strings.TrimSpace(c.Verify.HMACSecret)
	c.AWS.FromEmail = strings.TrimSpace(c.AWS.FromEmail)
	c.AWS.Region = strings.TrimSpace(c.AWS.Region)
	c.AWS.AccessKey = strings.TrimSpace(c.AWS.AccessKey)
	c.AWS.SecretKey = strings.TrimSpace(c.AWS.SecretKey)
	c.AWS.SESConfigurationSet = "dugble-transactional"
	c.AWS.SESTransactionalConfigurationSet = "dugble-transactional"
	c.AWS.SESMarketingConfigurationSet = "dugble-marketing"
	c.AWS.SESTenantName = "dugble-system"
	c.AWS.SNSTopicARNs = normalizeStrings(c.AWS.SNSTopicARNs)
	c.NATSURL = strings.TrimSpace(c.NATSURL)
	c.Arkesel.APIKey = strings.TrimSpace(c.Arkesel.APIKey)
	c.Arkesel.BaseURL = strings.TrimRight(strings.TrimSpace(c.Arkesel.BaseURL), "/")
	c.MNotify.APIKey = strings.TrimSpace(c.MNotify.APIKey)
	c.MNotify.BaseURL = strings.TrimRight(strings.TrimSpace(c.MNotify.BaseURL), "/")
	c.Celcom.APIKey = strings.TrimSpace(c.Celcom.APIKey)
	c.Celcom.PartnerID = strings.TrimSpace(c.Celcom.PartnerID)
	c.Celcom.BaseURL = strings.TrimRight(strings.TrimSpace(c.Celcom.BaseURL), "/")
	c.Backoffice.HTTPPort = strings.TrimSpace(c.Backoffice.HTTPPort)
	c.NewRelic.LicenseKey = strings.TrimSpace(c.NewRelic.LicenseKey)
	c.Sentry.DSN = strings.TrimSpace(c.Sentry.DSN)
	c.Sentry.Release = strings.TrimSpace(c.Sentry.Release)
	c.CORSOrigins = normalizeStrings(c.CORSOrigins)
	adminEmails := make([]string, 0, len(c.Backoffice.AdminEmails))
	for _, email := range c.Backoffice.AdminEmails {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			adminEmails = append(adminEmails, email)
		}
	}
	c.Backoffice.AdminEmails = adminEmails
}

func normalizeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
