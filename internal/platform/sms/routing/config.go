package routing

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

var (
	ErrNoRoutesConfigured = errors.New("no SMS routes configured")
	ErrNoEnabledRoutes    = errors.New("no SMS routes are enabled")
	ErrInvalidProviderID  = errors.New("invalid SMS provider ID")
	ErrInvalidCountryCode = errors.New("invalid SMS destination country")
	ErrInvalidPriority    = errors.New("invalid SMS provider priority")
	ErrDuplicateProvider  = errors.New("duplicate SMS provider")
	ErrDuplicatePriority  = errors.New("duplicate SMS provider priority")
)

type Route struct {
	ProviderID         string
	DestinationCountry string
	Priority           int
	Enabled            bool
}

type Config struct {
	Routes []Route
}

func DefaultConfig() Config {
	return Config{Routes: []Route{
		{
			ProviderID:         "mnotify",
			DestinationCountry: platformsms.CountryGhana,
			Priority:           1,
			Enabled:            true,
		},
		{
			ProviderID:         "moolre",
			DestinationCountry: platformsms.CountryGhana,
			Priority:           2,
			Enabled:            true,
		},
		{
			ProviderID:         "celcom",
			DestinationCountry: platformsms.CountryKenya,
			Priority:           1,
			Enabled:            true,
		},
		{
			ProviderID:         "arkesel",
			DestinationCountry: platformsms.CountryNigeria,
			Priority:           1,
			Enabled:            true,
		},
	}}
}

func (config Config) Validate() error {
	if len(config.Routes) == 0 {
		return ErrNoRoutesConfigured
	}

	providers := make(map[string]struct{}, len(config.Routes))
	priorities := make(map[string]string, len(config.Routes))
	enabledCount := 0

	for _, route := range config.Routes {
		providerID := normalizeProviderID(route.ProviderID)
		if providerID == "" {
			return ErrInvalidProviderID
		}

		country := platformsms.NormalizeCountryCode(route.DestinationCountry)
		if !platformsms.IsSupportedDestinationCountry(country) {
			return fmt.Errorf("%w for provider %q: %q", ErrInvalidCountryCode, providerID, route.DestinationCountry)
		}
		if route.Priority < 1 {
			return fmt.Errorf("%w for provider %q", ErrInvalidPriority, providerID)
		}

		providerKey := country + "\x00" + providerID
		if _, exists := providers[providerKey]; exists {
			return fmt.Errorf("%w for country %q: %s", ErrDuplicateProvider, country, providerID)
		}
		providers[providerKey] = struct{}{}

		priorityKey := fmt.Sprintf("%s\x00%d", country, route.Priority)
		if existingProvider, exists := priorities[priorityKey]; exists {
			return fmt.Errorf(
				"%w: providers %q and %q both use priority %d for country %q",
				ErrDuplicatePriority,
				existingProvider,
				providerID,
				route.Priority,
				country,
			)
		}
		priorities[priorityKey] = providerID

		if route.Enabled {
			enabledCount++
		}
	}

	if enabledCount == 0 {
		return ErrNoEnabledRoutes
	}
	return nil
}

// enabledRoutes returns a normalized, sorted copy. Callers can retain or
// modify the returned slice without mutating Config.
func (config Config) enabledRoutes() []Route {
	routes := make([]Route, 0, len(config.Routes))
	for _, route := range config.Routes {
		if !route.Enabled {
			continue
		}
		route.ProviderID = normalizeProviderID(route.ProviderID)
		route.DestinationCountry = platformsms.NormalizeCountryCode(route.DestinationCountry)
		routes = append(routes, route)
	}

	sort.SliceStable(routes, func(left, right int) bool {
		if routes[left].DestinationCountry != routes[right].DestinationCountry {
			return routes[left].DestinationCountry < routes[right].DestinationCountry
		}
		return routes[left].Priority < routes[right].Priority
	})
	return routes
}

func normalizeProviderID(providerID string) string {
	return strings.ToLower(strings.TrimSpace(providerID))
}
