package routing

import (
	"context"
	"errors"
	"fmt"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

var (
	ErrRoutingServiceNil     = errors.New("SMS routing service is nil")
	ErrStrategyRequired      = errors.New("SMS routing strategy is required")
	ErrProviderRequired      = errors.New("SMS provider is required")
	ErrProviderNotRegistered = errors.New("SMS provider is not registered")
)

type Service struct {
	routes    []Route
	strategy  Strategy
	providers map[string]platformsms.Provider
}

var _ platformsms.Router = (*Service)(nil)

func NewService(
	config Config,
	strategy Strategy,
	providers ...platformsms.Provider,
) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate SMS routing config: %w", err)
	}
	if strategy == nil {
		return nil, ErrStrategyRequired
	}

	routes := config.enabledRoutes()
	registry := make(map[string]platformsms.Provider, len(providers))
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
	request platformsms.SendRequest,
) ([]platformsms.Provider, error) {
	if service == nil {
		return nil, ErrRoutingServiceNil
	}
	if service.strategy == nil {
		return nil, ErrStrategyRequired
	}
	if ctx == nil {
		return nil, errors.New("SMS routing context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	request = request.Normalize()
	eligibleRoutes := make([]Route, 0, len(service.routes))
	for _, route := range service.routes {
		if route.DestinationCountry == request.DestinationCountry {
			eligibleRoutes = append(eligibleRoutes, route)
		}
	}
	if len(eligibleRoutes) == 0 {
		return nil, platformsms.ErrNoProviderAvailable
	}

	orderedRoutes := service.strategy.Order(ctx, request, eligibleRoutes)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	enabledProviders := make(map[string]struct{}, len(eligibleRoutes))
	for _, route := range eligibleRoutes {
		enabledProviders[route.ProviderID] = struct{}{}
	}

	result := make([]platformsms.Provider, 0, len(orderedRoutes))
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
		upstream, exists := service.providers[providerID]
		if !exists || upstream == nil {
			continue
		}
		seen[providerID] = struct{}{}
		result = append(result, upstream)
	}
	if len(result) == 0 {
		return nil, platformsms.ErrNoProviderAvailable
	}
	return result, nil
}

// Provider returns registered providers even if their routes are disabled, so
// delivery-status checks continue for messages accepted before a route change.
func (service *Service) Provider(providerID string) (platformsms.Provider, bool) {
	if service == nil {
		return nil, false
	}
	providerID = normalizeProviderID(providerID)
	if providerID == "" {
		return nil, false
	}
	upstream, exists := service.providers[providerID]
	return upstream, exists
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
