package arkesel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const ProviderID = "arkesel"

type Provider struct {
	client *Client
}

func NewProvider(client *Client) *Provider { return &Provider{client: client} }

func (provider *Provider) ID() string { return ProviderID }

func (provider *Provider) Send(ctx context.Context, request platformsms.SendRequest) (*platformsms.SendResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("Arkesel send failed: %w", ErrClientUnavailable)
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var response SendResponse
	if err := provider.client.do(ctx, http.MethodPost, "/api/v2/sms/send", newSendRequest(request), &response); err != nil {
		return nil, fmt.Errorf("Arkesel send failed: %w", err)
	}
	mapped, err := mapSendResponse(&response)
	if err != nil {
		return nil, fmt.Errorf("Arkesel send failed: %w", err)
	}
	return mapped, nil
}

func (provider *Provider) CheckStatus(ctx context.Context, messageID string) (*platformsms.StatusResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("Arkesel status check failed: %w", ErrClientUnavailable)
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("Arkesel status check failed: message ID is required")
	}
	var response StatusResponse
	if err := provider.client.do(ctx, http.MethodGet, "/api/v2/sms/"+url.PathEscape(messageID), nil, &response); err != nil {
		return nil, fmt.Errorf("Arkesel status check failed: %w", err)
	}
	mapped, err := mapStatusResponse(&response)
	if err != nil {
		return nil, fmt.Errorf("Arkesel status check failed: %w", err)
	}
	return mapped, nil
}
