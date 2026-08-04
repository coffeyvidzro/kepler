package routing

import (
	"errors"
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

var (
	ErrNoRoutesConfigured = errors.New("no SMS routes configured")
	ErrNoEnabledRoutes    = errors.New("no SMS routes are enabled")
	ErrInvalidProviderID  = errors.New("invalid SMS provider ID")
	ErrInvalidCountryCode = errors.New("invalid SMS destination country")
	ErrDuplicateProvider  = errors.New("duplicate SMS provider")
	ErrDuplicateCountry   = errors.New("duplicate SMS destination country")
)

type Route struct {
	ProviderID         string
	DestinationCountry string
	Enabled            bool
}

type Config struct {
	Routes []Route
}

func DefaultConfig() Config {
	return Config{
		Routes: []Route{
			{
				ProviderID:         "mnotify",
				DestinationCountry: sms.CountryGhana,
				Enabled:            true,
			},
			{
				ProviderID:         "celcom",
				DestinationCountry: sms.CountryKenya,
				Enabled:            true,
			},
			{
				ProviderID:         "arkesel",
				DestinationCountry: sms.CountryNigeria,
				Enabled:            true,
			},
		},
	}
}

func (c Config) Validate() error {
	if len(c.Routes) == 0 {
		return ErrNoRoutesConfigured
	}

	providers := make(map[string]string, len(c.Routes))
	countries := make(map[string]string, len(c.Routes))
	enabledCount := 0

	for _, route := range c.Routes {
		providerID := normalizeProviderID(route.ProviderID)
		if providerID == "" {
			return ErrInvalidProviderID
		}
		country := sms.NormalizeCountryCode(route.DestinationCountry)
		if !sms.IsCountryCode(country) {
			return fmt.Errorf("%w for provider %q: %q", ErrInvalidCountryCode, providerID, route.DestinationCountry)
		}

		if existingCountry, exists := providers[providerID]; exists {
			return fmt.Errorf("%w: %s is configured for %s and %s", ErrDuplicateProvider, providerID, existingCountry, country)
		}
		providers[providerID] = country

		if existingProvider, exists := countries[country]; exists {
			return fmt.Errorf("%w %q: providers %q and %q", ErrDuplicateCountry, country, existingProvider, providerID)
		}
		countries[country] = providerID

		if route.Enabled {
			enabledCount++
		}
	}

	if enabledCount == 0 {
		return ErrNoEnabledRoutes
	}
	return nil
}

func (c Config) enabledRoutes() []Route {
	routes := make([]Route, 0, len(c.Routes))
	for _, route := range c.Routes {
		if !route.Enabled {
			continue
		}
		route.ProviderID = normalizeProviderID(route.ProviderID)
		route.DestinationCountry = sms.NormalizeCountryCode(route.DestinationCountry)
		routes = append(routes, route)
	}
	return routes
}

func normalizeProviderID(providerID string) string {
	return strings.ToLower(strings.TrimSpace(providerID))
}
