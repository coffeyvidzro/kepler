package sms

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const (
	ProviderID = "mnotify"
	sendPath   = "/api/sms/quick"
	statusPath = "/api/campaign/"
)

type Provider struct{ client *mnotify.Client }

var _ platformsms.Provider = (*Provider)(nil)

func NewProvider(client *mnotify.Client) *Provider { return &Provider{client: client} }
func (provider *Provider) ID() string              { return ProviderID }

func (provider *Provider) Send(ctx context.Context, request platformsms.SendRequest) (*platformsms.SendResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("mNotify send failed: %w", mnotify.ErrClientUnavailable)
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}

	var response sendResponse
	if err := provider.client.Post(ctx, sendPath, newSendRequest(request), &response); err != nil {
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
		return nil, fmt.Errorf("mNotify status check failed: %w", mnotify.ErrClientUnavailable)
	}
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("mNotify status check failed: campaign ID is required")
	}

	var response statusResponse
	path := statusPath + url.PathEscape(campaignID)
	if err := provider.client.Get(ctx, path, &response); err != nil {
		return nil, fmt.Errorf("mNotify status check failed: %w", err)
	}
	mapped, err := mapStatusResponse(campaignID, &response)
	if err != nil {
		return nil, fmt.Errorf("mNotify status check failed: %w", err)
	}
	return mapped, nil
}
