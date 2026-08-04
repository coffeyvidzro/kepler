package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Service validates provider-neutral SMS requests and sends each message
// through the single provider configured for its destination country. Message
// persistence is handled by the application layer.
type Service struct {
	router Router
}

func NewService(router Router) (*Service, error) {
	if router == nil {
		return nil, ErrRouterRequired
	}
	return &Service{router: router}, nil
}

func (s *Service) Send(ctx context.Context, req SendRequest) (*SendResponse, error) {
	if s == nil || s.router == nil {
		return nil, ErrRouterRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	req = req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	upstream, err := s.router.Route(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("route SMS request: %w", err)
	}
	if upstream == nil {
		return nil, ErrNoProviderAvailable
	}

	providerID := strings.ToLower(strings.TrimSpace(upstream.ID()))
	if providerID == "" {
		return nil, &SendError{Attempts: []ProviderAttempt{{
			ProviderID: "unknown",
			Err:        errors.New("routed SMS provider has an empty ID"),
		}}}
	}

	response, err := upstream.Send(ctx, req)
	if err != nil {
		return nil, &SendError{Attempts: []ProviderAttempt{{ProviderID: providerID, Err: err}}}
	}
	if err := validateSendResponse(providerID, response); err != nil {
		return nil, &SendError{Attempts: []ProviderAttempt{{ProviderID: providerID, Err: err}}}
	}

	return response, nil
}

func (s *Service) CheckStatus(
	ctx context.Context,
	providerID string,
	providerMessageID string,
) (*StatusResponse, error) {
	if s == nil || s.router == nil {
		return nil, ErrRouterRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	providerID = strings.ToLower(strings.TrimSpace(providerID))
	providerMessageID = strings.TrimSpace(providerMessageID)

	if providerID == "" {
		return nil, &ValidationError{Field: "provider_id", Reason: "provider ID is required"}
	}
	if providerMessageID == "" {
		return nil, &ValidationError{
			Field:  "provider_message_id",
			Reason: "provider message ID is required",
		}
	}

	upstream, ok := s.router.Provider(providerID)
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

	response.ProviderID = strings.ToLower(strings.TrimSpace(response.ProviderID))
	response.ProviderMsgID = strings.TrimSpace(response.ProviderMsgID)
	response.Status = strings.ToLower(strings.TrimSpace(response.Status))

	if response.ProviderID == "" {
		return fmt.Errorf("%w: provider ID is empty", ErrInvalidProviderReply)
	}
	if response.ProviderID != expectedProviderID {
		return fmt.Errorf(
			"%w: provider ID %q does not match routed provider %q",
			ErrInvalidProviderReply,
			response.ProviderID,
			expectedProviderID,
		)
	}
	if response.ProviderMsgID == "" {
		return fmt.Errorf("%w: provider message ID is empty", ErrInvalidProviderReply)
	}
	if !IsKnownStatus(response.Status) {
		return fmt.Errorf(
			"%w: unsupported send status %q",
			ErrInvalidProviderReply,
			response.Status,
		)
	}

	return nil
}

func validateStatusResponse(
	expectedProviderID string,
	expectedProviderMessageID string,
	response *StatusResponse,
) error {
	if response == nil {
		return fmt.Errorf("%w: status response is nil", ErrInvalidProviderReply)
	}

	response.ProviderID = strings.ToLower(strings.TrimSpace(response.ProviderID))
	response.ProviderMsgID = strings.TrimSpace(response.ProviderMsgID)
	response.Status = strings.ToLower(strings.TrimSpace(response.Status))

	if response.ProviderID == "" {
		return fmt.Errorf("%w: provider ID is empty", ErrInvalidProviderReply)
	}
	if response.ProviderID != expectedProviderID {
		return fmt.Errorf(
			"%w: provider ID %q does not match requested provider %q",
			ErrInvalidProviderReply,
			response.ProviderID,
			expectedProviderID,
		)
	}
	if response.ProviderMsgID != expectedProviderMessageID {
		return fmt.Errorf(
			"%w: provider message ID %q does not match requested ID %q",
			ErrInvalidProviderReply,
			response.ProviderMsgID,
			expectedProviderMessageID,
		)
	}
	if !IsKnownStatus(response.Status) {
		return fmt.Errorf(
			"%w: unsupported delivery status %q",
			ErrInvalidProviderReply,
			response.Status,
		)
	}

	return nil
}
