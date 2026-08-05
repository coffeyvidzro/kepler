package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"

	platformrouting "github.com/coffeyvidzro/dugble/server/internal/platform/sms/routing"
)

type Route = platformrouting.Route
type RoutingConfig = platformrouting.Config

func DefaultRoutingConfig() RoutingConfig {
	return platformrouting.DefaultConfig()
}

type RoutingService struct {
	service   *platformrouting.Service
	providers map[string]Provider
}

var _ Router = (*RoutingService)(nil)

func NewRoutingService(
	config RoutingConfig,
	providers ...Provider,
) (*RoutingService, error) {
	registry := make(map[string]Provider, len(providers))
	providerIDs := make([]string, 0, len(providers))
	for _, upstream := range providers {
		if upstream == nil {
			return nil, platformrouting.ErrProviderRequired
		}
		providerID := normalizeProviderID(upstream.ID())
		if providerID == "" {
			return nil, platformrouting.ErrInvalidProviderID
		}
		if _, exists := registry[providerID]; exists {
			return nil, fmt.Errorf("%w: %s", platformrouting.ErrDuplicateProvider, providerID)
		}
		registry[providerID] = upstream
		providerIDs = append(providerIDs, providerID)
	}

	service, err := platformrouting.NewService(
		config,
		platformrouting.NewPriorityStrategy(),
		providerIDs...,
	)
	if err != nil {
		return nil, err
	}
	return &RoutingService{service: service, providers: registry}, nil
}

func (service *RoutingService) Route(
	ctx context.Context,
	request SendRequest,
) ([]Provider, error) {
	if service == nil || service.service == nil {
		return nil, platformrouting.ErrRoutingServiceNil
	}
	providerIDs, err := service.service.Route(ctx, request.Normalize())
	if err != nil {
		if errors.Is(err, platformrouting.ErrNoProviderAvailable) {
			return nil, ErrNoProviderAvailable
		}
		return nil, err
	}

	providers := make([]Provider, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		upstream, exists := service.providers[providerID]
		if !exists || upstream == nil {
			continue
		}
		providers = append(providers, upstream)
	}
	if len(providers) == 0 {
		return nil, ErrNoProviderAvailable
	}
	return providers, nil
}

func (service *RoutingService) Provider(providerID string) (Provider, bool) {
	if service == nil {
		return nil, false
	}
	upstream, exists := service.providers[normalizeProviderID(providerID)]
	return upstream, exists
}

func (service *RoutingService) ShouldFallback(
	ctx context.Context,
	providerID string,
	err error,
) bool {
	if service == nil || service.service == nil {
		return false
	}
	return service.service.ShouldFallback(ctx, providerID, err)
}

func normalizeProviderID(providerID string) string {
	return strings.ToLower(strings.TrimSpace(providerID))
}
