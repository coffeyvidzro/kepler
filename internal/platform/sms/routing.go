package sms

import (
	"context"
	"fmt"
	"strings"
)

type Route struct {
	ProviderID         string
	DestinationCountry string
	Enabled            bool
}

type RoutingConfig struct {
	Routes []Route
}

func DefaultRoutingConfig() RoutingConfig {
	return RoutingConfig{Routes: []Route{
		{ProviderID: "mnotify", DestinationCountry: CountryGhana, Enabled: true},
		{ProviderID: "celcom", DestinationCountry: CountryKenya, Enabled: true},
		{ProviderID: "arkesel", DestinationCountry: CountryNigeria, Enabled: true},
	}}
}

func (config RoutingConfig) Validate() error {
	if len(config.Routes) == 0 {
		return ErrNoRoutesConfigured
	}
	providers := make(map[string]string, len(config.Routes))
	countries := make(map[string]string, len(config.Routes))
	enabled := 0
	for _, route := range config.Routes {
		providerID := normalizeProviderID(route.ProviderID)
		if providerID == "" {
			return ErrInvalidProviderID
		}
		country := NormalizeCountryCode(route.DestinationCountry)
		if !IsCountryCode(country) {
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
			enabled++
		}
	}
	if enabled == 0 {
		return ErrNoEnabledRoutes
	}
	return nil
}

type RoutingService struct {
	routes    map[string]string
	providers map[string]Provider
}

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
	routes := make(map[string]string)
	for _, route := range config.Routes {
		if !route.Enabled {
			continue
		}
		providerID := normalizeProviderID(route.ProviderID)
		country := NormalizeCountryCode(route.DestinationCountry)
		if _, exists := registry[providerID]; !exists {
			return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, providerID)
		}
		routes[country] = providerID
	}
	return &RoutingService{routes: routes, providers: registry}, nil
}

func (service *RoutingService) Route(ctx context.Context, request SendRequest) (Provider, error) {
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
	providerID, exists := service.routes[request.DestinationCountry]
	if !exists {
		return nil, ErrNoProviderAvailable
	}
	upstream, exists := service.providers[providerID]
	if !exists || upstream == nil {
		return nil, ErrNoProviderAvailable
	}
	return upstream, nil
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
