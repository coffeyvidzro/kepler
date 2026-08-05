package sms

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type Route struct {
	ProviderID         string
	DestinationCountry string
	Priority           int
	Enabled            bool
}

type RoutingConfig struct {
	Routes []Route
}

func DefaultRoutingConfig() RoutingConfig {
	return RoutingConfig{Routes: []Route{
		{ProviderID: "mnotify", DestinationCountry: CountryGhana, Priority: 1, Enabled: true},
		{ProviderID: "celcom", DestinationCountry: CountryKenya, Priority: 1, Enabled: true},
		{ProviderID: "arkesel", DestinationCountry: CountryNigeria, Priority: 1, Enabled: true},
	}}
}

func (config RoutingConfig) Validate() error {
	if len(config.Routes) == 0 {
		return ErrNoRoutesConfigured
	}

	providerRoutes := make(map[string]struct{}, len(config.Routes))
	countryPriorities := make(map[string]struct{}, len(config.Routes))
	enabled := 0

	for _, route := range config.Routes {
		providerID := normalizeProviderID(route.ProviderID)
		if providerID == "" {
			return ErrInvalidProviderID
		}

		country := NormalizeCountryCode(route.DestinationCountry)
		if !IsSupportedDestinationCountry(country) {
			return fmt.Errorf("%w for provider %q: %q", ErrInvalidCountryCode, providerID, route.DestinationCountry)
		}
		if route.Priority <= 0 {
			return fmt.Errorf("%w for provider %q in %s: %d", ErrInvalidRoutePriority, providerID, country, route.Priority)
		}

		providerRouteKey := country + "\x00" + providerID
		if _, exists := providerRoutes[providerRouteKey]; exists {
			return fmt.Errorf("%w: provider %q for country %q", ErrDuplicateRoute, providerID, country)
		}
		providerRoutes[providerRouteKey] = struct{}{}

		priorityKey := fmt.Sprintf("%s\x00%d", country, route.Priority)
		if _, exists := countryPriorities[priorityKey]; exists {
			return fmt.Errorf("%w: country %q priority %d", ErrDuplicateRoutePriority, country, route.Priority)
		}
		countryPriorities[priorityKey] = struct{}{}

		if route.Enabled {
			enabled++
		}
	}

	if enabled == 0 {
		return ErrNoEnabledRoutes
	}
	return nil
}

type registeredRoute struct {
	providerID string
	priority   int
}

type RoutingService struct {
	routes    map[string][]registeredRoute
	providers map[string]Provider
}

var _ CandidateRouter = (*RoutingService)(nil)

func NewRoutingService(config RoutingConfig, providers ...Provider) (*RoutingService, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate SMS routing config: %w", err)
	}

	registry := make(map[string]Provider, len(providers))
	for _, upstream := range providers {
		if upstream == nil {
			return nil, ErrProviderRequired
		}
		providerID := normalizeProviderID(upstream.ID())
		if providerID == "" {
			return nil, ErrInvalidProviderID
		}
		if _, exists := registry[providerID]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateProvider, providerID)
		}
		registry[providerID] = upstream
	}

	routes := make(map[string][]registeredRoute)
	for _, route := range config.Routes {
		if !route.Enabled {
			continue
		}
		providerID := normalizeProviderID(route.ProviderID)
		country := NormalizeCountryCode(route.DestinationCountry)
		if _, exists := registry[providerID]; !exists {
			return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, providerID)
		}
		routes[country] = append(routes[country], registeredRoute{
			providerID: providerID,
			priority:   route.Priority,
		})
	}

	for country := range routes {
		sort.Slice(routes[country], func(left, right int) bool {
			return routes[country][left].priority < routes[country][right].priority
		})
	}

	return &RoutingService{routes: routes, providers: registry}, nil
}

func (service *RoutingService) Route(ctx context.Context, request SendRequest) (Provider, error) {
	candidates, err := service.Candidates(ctx, request)
	if err != nil {
		return nil, err
	}
	return candidates[0], nil
}

func (service *RoutingService) Candidates(ctx context.Context, request SendRequest) ([]Provider, error) {
	if service == nil {
		return nil, ErrRoutingServiceNil
	}
	if ctx == nil {
		return nil, fmt.Errorf("SMS routing context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	request = request.Normalize()
	configuredRoutes := service.routes[request.DestinationCountry]
	if len(configuredRoutes) == 0 {
		return nil, ErrNoProviderAvailable
	}

	candidates := make([]Provider, 0, len(configuredRoutes))
	for _, route := range configuredRoutes {
		upstream, exists := service.providers[route.providerID]
		if !exists || upstream == nil {
			continue
		}
		candidates = append(candidates, upstream)
	}
	if len(candidates) == 0 {
		return nil, ErrNoProviderAvailable
	}
	return candidates, nil
}

func (service *RoutingService) Provider(providerID string) (Provider, bool) {
	if service == nil {
		return nil, false
	}
	upstream, exists := service.providers[normalizeProviderID(providerID)]
	return upstream, exists
}

func normalizeProviderID(providerID string) string {
	return strings.ToLower(strings.TrimSpace(providerID))
}
