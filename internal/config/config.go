package config

import (
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type ProviderConfig struct {
	APIKey string `env:"API_KEY"`
}

type MoolreConfig struct {
	VASKey string `env:"VAS_KEY"`
}

type ArgusConfig struct {
	HMACSecret string `env:"HMAC_SECRET"`
	SenderID   string `env:"SENDER_ID" envDefault:"Dugble"`
	FromEmail  string `env:"FROM_EMAIL" envDefault:"argus@dugble.com"`
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
	AppEnv         string         `env:"APP_ENV"   envDefault:"development"`
	HTTPPort       string         `env:"HTTP_PORT" envDefault:"8080"`
	DatabaseURL    string         `env:"DATABASE_URL,required,notEmpty"`
	RedisURL       string         `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`
	CORSOrigins    []string       `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000,http://127.0.0.1:3000"`
	ArcjetKey      string         `env:"ARCJET_KEY,required,notEmpty"`
	FrontendURL    string         `env:"FRONTEND_URL" envDefault:"http://localhost:3000"`
	BackendURL     string         `env:"BACKEND_URL" envDefault:"http://localhost:8080"`
	CookieDomain   string         `env:"COOKIE_DOMAIN"`
	EncryptionKeys []string       `env:"ENCRYPTION_KEYS" envSeparator:","`
	Argus          ArgusConfig    `envPrefix:"ARGUS_"`
	AWS            AWSConfig      `envPrefix:"AWS_"`
	NATSURL        string         `env:"NATS_URL" envDefault:"nats://localhost:4222"`
	MNotify        ProviderConfig `envPrefix:"MNOTIFY_"`
	Moolre         MoolreConfig   `envPrefix:"MOOLRE_"`
	NewRelic       NewRelicConfig `envPrefix:"NEW_RELIC_"`
	Sentry         SentryConfig   `envPrefix:"SENTRY_"`
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
	c.Argus.HMACSecret = strings.TrimSpace(c.Argus.HMACSecret)
	c.Argus.SenderID = strings.TrimSpace(c.Argus.SenderID)
	c.Argus.FromEmail = strings.ToLower(strings.TrimSpace(c.Argus.FromEmail))
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
	c.MNotify.APIKey = strings.TrimSpace(c.MNotify.APIKey)
	c.Moolre.VASKey = strings.TrimSpace(c.Moolre.VASKey)
	c.NewRelic.LicenseKey = strings.TrimSpace(c.NewRelic.LicenseKey)
	c.Sentry.DSN = strings.TrimSpace(c.Sentry.DSN)
	c.Sentry.Release = strings.TrimSpace(c.Sentry.Release)
	c.CORSOrigins = normalizeStrings(c.CORSOrigins)
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
