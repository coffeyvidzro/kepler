package sms

import (
	"context"
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const (
	ProviderID = "moolre"
	sendPath   = "/open/sms/send"
	statusPath = "/open/sms/status"
)

type Provider struct{ client *moolre.Client }

var _ platformsms.Provider = (*Provider)(nil)

func NewProvider(client *moolre.Client) *Provider { return &Provider{client: client} }
func (provider *Provider) ID() string             { return ProviderID }

func (provider *Provider) Send(ctx context.Context, request platformsms.SendRequest) (*platformsms.SendResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("moolre SMS send failed: %w", moolre.ErrClientUnavailable)
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.Reference == "" {
		return nil, &platformsms.ValidationError{Field: "reference", Reason: "message reference is required"}
	}

	var response sendResponse
	if err := provider.client.Post(ctx, sendPath, newSendRequest(request), &response); err != nil {
		return nil, fmt.Errorf("moolre SMS send failed: %w", err)
	}
	mapped, err := mapSendResponse(request.Reference, &response)
	if err != nil {
		return nil, fmt.Errorf("moolre SMS send failed: %w", err)
	}
	return mapped, nil
}

func (provider *Provider) CheckStatus(ctx context.Context, reference string) (*platformsms.StatusResponse, error) {
	statuses, err := provider.CheckStatuses(ctx, []string{reference})
	if err != nil {
		return nil, err
	}
	return &statuses[0], nil
}

func (provider *Provider) CheckStatuses(ctx context.Context, references []string) ([]platformsms.StatusResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("moolre SMS status check failed: %w", moolre.ErrClientUnavailable)
	}
	normalized, err := normalizeReferences(references)
	if err != nil {
		return nil, fmt.Errorf("moolre SMS status check failed: %w", err)
	}

	var response statusResponse
	if err := provider.client.Post(ctx, statusPath, newStatusRequest(normalized), &response); err != nil {
		return nil, fmt.Errorf("moolre SMS status check failed: %w", err)
	}
	mapped, err := mapStatusResponse(normalized, &response)
	if err != nil {
		return nil, fmt.Errorf("moolre SMS status check failed: %w", err)
	}
	return mapped, nil
}

func normalizeReferences(references []string) ([]string, error) {
	if len(references) == 0 {
		return nil, fmt.Errorf("at least one SMS reference is required")
	}
	result := make([]string, 0, len(references))
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		reference = strings.TrimSpace(reference)
		if reference == "" {
			return nil, fmt.Errorf("SMS reference is required")
		}
		if _, exists := seen[reference]; exists {
			return nil, fmt.Errorf("duplicate SMS reference %q", reference)
		}
		seen[reference] = struct{}{}
		result = append(result, reference)
	}
	return result, nil
}
