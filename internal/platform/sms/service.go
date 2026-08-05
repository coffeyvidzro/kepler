package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Service struct {
	router Router
}

func NewService(router Router) (*Service, error) {
	if router == nil {
		return nil, ErrRouterRequired
	}
	return &Service{router: router}, nil
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

	candidates, err := service.routeCandidates(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("route SMS request: %w", err)
	}
	if len(candidates) == 0 {
		return nil, ErrNoProviderAvailable
	}

	attempts := make([]ProviderAttempt, 0, len(candidates))
	for _, upstream := range candidates {
		if upstream == nil {
			attempts = append(attempts, ProviderAttempt{
				ProviderID: "unknown",
				Err:        errors.New("routed SMS provider is nil"),
			})
			break
		}

		providerID := normalizeProviderID(upstream.ID())
		if providerID == "" {
			attempts = append(attempts, ProviderAttempt{
				ProviderID: "unknown",
				Err:        errors.New("routed SMS provider has an empty ID"),
			})
			break
		}

		response, attemptErr := upstream.Send(ctx, request)
		if attemptErr == nil {
			attemptErr = validateSendResponse(providerID, response)
			if attemptErr == nil {
				return response, nil
			}
		}

		attempts = append(attempts, ProviderAttempt{
			ProviderID: providerID,
			Err:        attemptErr,
		})
		if !safeToFallback(attemptErr) {
			break
		}
	}

	return nil, &SendError{Attempts: attempts}
}

func (service *Service) routeCandidates(ctx context.Context, request SendRequest) ([]Provider, error) {
	if router, ok := service.router.(CandidateRouter); ok {
		return router.Candidates(ctx, request)
	}
	upstream, err := service.router.Route(ctx, request)
	if err != nil {
		return nil, err
	}
	if upstream == nil {
		return nil, ErrNoProviderAvailable
	}
	return []Provider{upstream}, nil
}

func safeToFallback(err error) bool {
	if err == nil {
		return false
	}
	var classifier interface {
		SafeToFallback() bool
	}
	return errors.As(err, &classifier) && classifier.SafeToFallback()
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
	upstream, ok := service.router.Provider(providerID)
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
