package sms

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	providers []Provider
}

var _ Router = (*RoutingService)(nil)

func NewRoutingService(
	config RoutingConfig,
	providers ...Provider,
) (*RoutingService, error) {
	registry, normalizedProviders, err := providerRegistry(providers)
	if err != nil {
		return nil, err
	}

	service, err := platformrouting.NewService(
		config,
		platformrouting.NewPriorityStrategy(),
		IsSupportedDestinationCountry,
	)
	if err != nil {
		return nil, err
	}
	for _, providerID := range service.ProviderIDs() {
		if _, exists := registry[providerID]; !exists {
			return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, providerID)
		}
	}

	return &RoutingService{service: service, providers: normalizedProviders}, nil
}

func (service *RoutingService) Route(
	ctx context.Context,
	destinationCountry string,
) ([]string, error) {
	if service == nil || service.service == nil {
		return nil, platformrouting.ErrRoutingServiceNil
	}
	return service.service.Route(ctx, destinationCountry)
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

func (service *RoutingService) ProviderIDs() []string {
	if service == nil || service.service == nil {
		return nil
	}
	return service.service.ProviderIDs()
}

func (service *RoutingService) Providers() []Provider {
	if service == nil {
		return nil
	}
	result := make([]Provider, len(service.providers))
	copy(result, service.providers)
	return result
}

type Service struct {
	router    Router
	providers map[string]Provider
}

type routedProviderSource interface {
	ProviderIDs() []string
}

type providerSource interface {
	Providers() []Provider
}

func NewService(router Router, providers ...Provider) (*Service, error) {
	if router == nil {
		return nil, ErrRouterRequired
	}
	if len(providers) == 0 {
		if source, ok := router.(providerSource); ok {
			providers = source.Providers()
		}
	}

	registry, _, err := providerRegistry(providers)
	if err != nil {
		return nil, err
	}
	if source, ok := router.(routedProviderSource); ok {
		for _, providerID := range source.ProviderIDs() {
			providerID = normalizeProviderID(providerID)
			if _, exists := registry[providerID]; !exists {
				return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, providerID)
			}
		}
	}

	return &Service{router: router, providers: registry}, nil
}

func (service *Service) Send(ctx context.Context, request SendRequest) (*SendResponse, error) {
	if service == nil || service.router == nil {
		return nil, ErrRouterRequired
	}
	if ctx == nil {
		return nil, errors.New("SMS send context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}

	providerIDs, err := service.router.Route(ctx, request.DestinationCountry)
	if err != nil {
		return nil, fmt.Errorf("route SMS request: %w", err)
	}
	if len(providerIDs) == 0 {
		return nil, ErrNoProviderAvailable
	}

	attempts := make([]ProviderAttempt, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		response, attemptErr := service.SendWithProvider(ctx, providerID, request)
		if attemptErr == nil {
			return response, nil
		}
		providerID = normalizeProviderID(providerID)
		attempts = append(attempts, ProviderAttempt{ProviderID: providerID, Err: attemptErr})
		if !service.ShouldFallback(ctx, providerID, attemptErr) {
			break
		}
	}
	return nil, &SendError{Attempts: attempts}
}

func (service *Service) SendWithProvider(
	ctx context.Context,
	providerID string,
	request SendRequest,
) (*SendResponse, error) {
	if service == nil || service.router == nil {
		return nil, ErrRouterRequired
	}
	if ctx == nil {
		return nil, errors.New("SMS send context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}
	providerID = normalizeProviderID(providerID)
	upstream, exists := service.providers[providerID]
	if providerID == "" || !exists || upstream == nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}
	response, err := upstream.Send(ctx, request)
	if err != nil {
		return nil, err
	}
	if err := validateSendResponse(providerID, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (service *Service) ShouldFallback(ctx context.Context, providerID string, err error) bool {
	if service == nil || service.router == nil {
		return false
	}
	return service.router.ShouldFallback(ctx, providerID, err)
}

func (service *Service) ProviderIDs() []string {
	if service == nil {
		return nil
	}
	providerIDs := make([]string, 0, len(service.providers))
	for providerID := range service.providers {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Strings(providerIDs)
	return providerIDs
}

func (service *Service) CheckStatus(ctx context.Context, providerID, providerMessageID string) (*StatusResponse, error) {
	if service == nil || service.router == nil {
		return nil, ErrRouterRequired
	}
	if ctx == nil {
		return nil, errors.New("SMS status context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	providerID = normalizeProviderID(providerID)
	providerMessageID = strings.TrimSpace(providerMessageID)
	if providerID == "" {
		return nil, &ValidationError{Field: "provider_id", Reason: "provider ID is required"}
	}
	if providerMessageID == "" {
		return nil, &ValidationError{Field: "provider_message_id", Reason: "provider message ID is required"}
	}
	upstream, ok := service.providers[providerID]
	if !ok || upstream == nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, providerID)
	}
	response, err := upstream.CheckStatus(ctx, providerMessageID)
	if err != nil {
		return nil, fmt.Errorf("check %s SMS status: %w", providerID, err)
	}
	if err := validateStatusResponse(providerID, providerMessageID, response); err != nil {
		return nil, err
	}
	return response, nil
}

func providerRegistry(providers []Provider) (map[string]Provider, []Provider, error) {
	if len(providers) == 0 {
		return nil, nil, ErrProviderRequired
	}
	registry := make(map[string]Provider, len(providers))
	normalizedProviders := make([]Provider, 0, len(providers))
	for _, upstream := range providers {
		if upstream == nil {
			return nil, nil, ErrProviderRequired
		}
		providerID := normalizeProviderID(upstream.ID())
		if providerID == "" {
			return nil, nil, ErrInvalidProviderID
		}
		if _, exists := registry[providerID]; exists {
			return nil, nil, fmt.Errorf("%w: %s", ErrDuplicateProvider, providerID)
		}
		registry[providerID] = upstream
		normalizedProviders = append(normalizedProviders, upstream)
	}
	return registry, normalizedProviders, nil
}

func validateSendResponse(expectedProviderID string, response *SendResponse) error {
	if response == nil {
		return fmt.Errorf("%w: send response is nil", ErrInvalidProviderReply)
	}
	response.ProviderID = normalizeProviderID(response.ProviderID)
	response.ProviderMsgID = strings.TrimSpace(response.ProviderMsgID)
	response.Status = strings.ToLower(strings.TrimSpace(response.Status))
	if response.ProviderID == "" {
		return fmt.Errorf("%w: provider ID is empty", ErrInvalidProviderReply)
	}
	if response.ProviderID != expectedProviderID {
		return fmt.Errorf("%w: provider ID %q does not match routed provider %q", ErrInvalidProviderReply, response.ProviderID, expectedProviderID)
	}
	if response.ProviderMsgID == "" {
		return fmt.Errorf("%w: provider message ID is empty", ErrInvalidProviderReply)
	}
	if !IsKnownStatus(response.Status) {
		return fmt.Errorf("%w: unsupported send status %q", ErrInvalidProviderReply, response.Status)
	}
	return nil
}

func validateStatusResponse(expectedProviderID, expectedProviderMessageID string, response *StatusResponse) error {
	if response == nil {
		return fmt.Errorf("%w: status response is nil", ErrInvalidProviderReply)
	}
	response.ProviderID = normalizeProviderID(response.ProviderID)
	response.ProviderMsgID = strings.TrimSpace(response.ProviderMsgID)
	response.Status = strings.ToLower(strings.TrimSpace(response.Status))
	if response.ProviderID == "" {
		return fmt.Errorf("%w: provider ID is empty", ErrInvalidProviderReply)
	}
	if response.ProviderID != expectedProviderID {
		return fmt.Errorf("%w: provider ID %q does not match requested provider %q", ErrInvalidProviderReply, response.ProviderID, expectedProviderID)
	}
	if response.ProviderMsgID != expectedProviderMessageID {
		return fmt.Errorf("%w: provider message ID %q does not match requested ID %q", ErrInvalidProviderReply, response.ProviderMsgID, expectedProviderMessageID)
	}
	if !IsKnownStatus(response.Status) {
		return fmt.Errorf("%w: unsupported delivery status %q", ErrInvalidProviderReply, response.Status)
	}
	return nil
}

func normalizeProviderID(providerID string) string {
	return strings.ToLower(strings.TrimSpace(providerID))
}
