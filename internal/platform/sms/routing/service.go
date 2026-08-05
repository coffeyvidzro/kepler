package routing

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrRoutingServiceNil        = errors.New("SMS routing service is nil")
	ErrStrategyRequired         = errors.New("SMS routing strategy is required")
	ErrCountryValidatorRequired = errors.New("SMS destination country validator is required")
	ErrUnsupportedCountry       = errors.New("unsupported SMS destination country")
	ErrNoProviderAvailable      = errors.New("no SMS provider is available")
)

type Service struct {
	routes             []Route
	strategy           Strategy
	isSupportedCountry CountryValidator
}

func NewService(
	config Config,
	strategy Strategy,
	isSupportedCountry CountryValidator,
) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate SMS routing config: %w", err)
	}
	if strategy == nil {
		return nil, ErrStrategyRequired
	}
	if isSupportedCountry == nil {
		return nil, ErrCountryValidatorRequired
	}

	for _, route := range config.Routes {
		country := normalizeCountryCode(route.DestinationCountry)
		if !isSupportedCountry(country) {
			return nil, fmt.Errorf(
				"%w for provider %q: %q",
				ErrUnsupportedCountry,
				normalizeProviderID(route.ProviderID),
				route.DestinationCountry,
			)
		}
	}

	return &Service{
		routes:             config.enabledRoutes(),
		strategy:           strategy,
		isSupportedCountry: isSupportedCountry,
	}, nil
}

func (service *Service) Route(
	ctx context.Context,
	destinationCountry string,
) ([]string, error) {
	if service == nil {
		return nil, ErrRoutingServiceNil
	}
	if service.strategy == nil {
		return nil, ErrStrategyRequired
	}
	if service.isSupportedCountry == nil {
		return nil, ErrCountryValidatorRequired
	}
	if ctx == nil {
		return nil, errors.New("SMS routing context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	country := normalizeCountryCode(destinationCountry)
	if !isCountryCode(country) || !service.isSupportedCountry(country) {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedCountry, destinationCountry)
	}

	eligibleRoutes := make([]Route, 0, len(service.routes))
	for _, route := range service.routes {
		if route.DestinationCountry == country {
			eligibleRoutes = append(eligibleRoutes, route)
		}
	}
	if len(eligibleRoutes) == 0 {
		return nil, ErrNoProviderAvailable
	}

	orderedRoutes := service.strategy.Order(ctx, country, eligibleRoutes)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	enabledProviders := make(map[string]struct{}, len(eligibleRoutes))
	for _, route := range eligibleRoutes {
		enabledProviders[route.ProviderID] = struct{}{}
	}

	result := make([]string, 0, len(orderedRoutes))
	seen := make(map[string]struct{}, len(orderedRoutes))
	for _, route := range orderedRoutes {
		providerID := normalizeProviderID(route.ProviderID)
		if providerID == "" {
			continue
		}
		if _, allowed := enabledProviders[providerID]; !allowed {
			continue
		}
		if _, exists := seen[providerID]; exists {
			continue
		}
		seen[providerID] = struct{}{}
		result = append(result, providerID)
	}
	if len(result) == 0 {
		return nil, ErrNoProviderAvailable
	}
	return result, nil
}

func (service *Service) ProviderIDs() []string {
	if service == nil {
		return nil
	}
	result := make([]string, 0, len(service.routes))
	seen := make(map[string]struct{}, len(service.routes))
	for _, route := range service.routes {
		if _, exists := seen[route.ProviderID]; exists {
			continue
		}
		seen[route.ProviderID] = struct{}{}
		result = append(result, route.ProviderID)
	}
	return result
}

func (service *Service) ShouldFallback(
	ctx context.Context,
	providerID string,
	err error,
) bool {
	if service == nil || service.strategy == nil || err == nil || ctx == nil || ctx.Err() != nil {
		return false
	}
	providerID = normalizeProviderID(providerID)
	if providerID == "" {
		return false
	}
	return service.strategy.ShouldFallback(ctx, providerID, err)
}
