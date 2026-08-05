package mnotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const ProviderID = "mnotify"

type Provider struct{ client *Client }

func NewProvider(client *Client) *Provider { return &Provider{client: client} }
func (provider *Provider) ID() string      { return ProviderID }

func (provider *Provider) Send(ctx context.Context, request platformsms.SendRequest) (*platformsms.SendResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("mNotify send failed: %w", ErrClientUnavailable)
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}
	var response SendResponse
	if err := provider.client.do(ctx, http.MethodPost, "/api/sms/quick", newSendRequest(request), &response); err != nil {
		return nil, fmt.Errorf("mNotify send failed: %w", err)
	}
	mapped, err := mapSendResponse(&response)
	if err != nil {
		return nil, fmt.Errorf("mNotify send failed: %w", err)
	}
	return mapped, nil
}

func (provider *Provider) CheckStatus(ctx context.Context, campaignID string) (*platformsms.StatusResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("mNotify status check failed: %w", ErrClientUnavailable)
	}
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("mNotify status check failed: campaign ID is required")
	}
	var response CampaignStatusResponse
	if err := provider.client.do(ctx, http.MethodGet, "/api/campaign/"+url.PathEscape(campaignID), nil, &response); err != nil {
		return nil, fmt.Errorf("mNotify status check failed: %w", err)
	}
	mapped, err := mapCampaignStatusResponse(campaignID, &response)
	if err != nil {
		return nil, fmt.Errorf("mNotify status check failed: %w", err)
	}
	return mapped, nil
}
