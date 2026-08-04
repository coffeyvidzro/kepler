package routing

import (
	"context"
	"errors"
	"fmt"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider"
)

var (
	ErrRoutingServiceNil     = errors.New("SMS routing service is nil")
	ErrProviderRequired      = errors.New("SMS provider is required")
	ErrProviderNotRegistered = errors.New("SMS provider is not registered")
)

type Service struct {
	routes    map[string]string
	providers map[string]provider.Provider
}

func NewService(config Config, providers ...provider.Provider) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate SMS routing config: %w", err)
	}

	registry := make(map[string]provider.Provider, len(providers))
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
	for _, route := range config.enabledRoutes() {
		if _, exists := registry[route.ProviderID]; !exists {
			return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, route.ProviderID)
		}
		routes[route.DestinationCountry] = route.ProviderID
	}

	return &Service{routes: routes, providers: registry}, nil
}

func (s *Service) Route(ctx context.Context, req sms.SendRequest) (sms.Provider, error) {
	if s == nil {
		return nil, ErrRoutingServiceNil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	req = req.Normalize()
	providerID, exists := s.routes[req.DestinationCountry]
	if !exists {
		return nil, sms.ErrNoProviderAvailable
	}

	upstream, exists := s.providers[providerID]
	if !exists || upstream == nil {
		return nil, sms.ErrNoProviderAvailable
	}
	return upstream, nil
}

// Provider returns a registered provider even when its route is currently
// disabled. This allows delivery-status checks for messages accepted before a
// provider was disabled.
func (s *Service) Provider(providerID string) (sms.Provider, bool) {
	if s == nil {
		return nil, false
	}
	providerID = normalizeProviderID(providerID)
	if providerID == "" {
		return nil, false
	}
	upstream, exists := s.providers[providerID]
	return upstream, exists
}
