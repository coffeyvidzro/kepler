package routing

import (
	"context"
	"errors"
	"fmt"

	"github.com/coffeyvidzro/dugble/server/internal/platform/sms/destination"
)

var (
	ErrRoutingServiceNil     = errors.New("SMS routing service is nil")
	ErrStrategyRequired      = errors.New("SMS routing strategy is required")
	ErrProviderRequired      = errors.New("SMS provider is required")
	ErrProviderNotRegistered = errors.New("SMS provider is not registered")
	ErrNoProviderAvailable   = errors.New("no SMS provider is available")
)

type Service struct {
	routes    []Route
	strategy  Strategy
	providers map[string]struct{}
}

func NewService(
	config Config,
	strategy Strategy,
	providerIDs ...string,
) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate SMS routing config: %w", err)
	}
	if strategy == nil {
		return nil, ErrStrategyRequired
	}

	routes := config.enabledRoutes()
	registry := make(map[string]struct{}, len(providerIDs))
	for _, rawProviderID := range providerIDs {
		providerID := normalizeProviderID(rawProviderID)
		if providerID == "" {
			return nil, ErrProviderRequired
		}
		if _, exists := registry[providerID]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateProvider, providerID)
		}
		registry[providerID] = struct{}{}
	}

	for _, route := range routes {
		if _, exists := registry[route.ProviderID]; !exists {
			return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, route.ProviderID)
		}
	}

	return &Service{
		routes:    routes,
		strategy:  strategy,
		providers: registry,
	}, nil
}

func (service *Service) Route(
	ctx context.Context,
	request Request,
) ([]string, error) {
	if service == nil {
		return nil, ErrRoutingServiceNil
	}
	if service.strategy == nil {
		return nil, ErrStrategyRequired
	}
	if ctx == nil {
		return nil, errors.New("SMS routing context is required")
	}
	if request == nil {
		return nil, errors.New("SMS routing request is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	country := destination.NormalizeCountryCode(request.RoutingCountry())
	eligibleRoutes := make([]Route, 0, len(service.routes))
	for _, route := range service.routes {
		if route.DestinationCountry == country {
			eligibleRoutes = append(eligibleRoutes, route)
		}
	}
	if len(eligibleRoutes) == 0 {
		return nil, ErrNoProviderAvailable
	}

	orderedRoutes := service.strategy.Order(ctx, request, eligibleRoutes)
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
		if _, registered := service.providers[providerID]; !registered {
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

func (service *Service) HasProvider(providerID string) bool {
	if service == nil {
		return false
	}
	_, exists := service.providers[normalizeProviderID(providerID)]
	return exists
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
