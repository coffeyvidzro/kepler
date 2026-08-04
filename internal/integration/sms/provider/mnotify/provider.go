package mnotify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

type Provider struct {
	client *Client
}

func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

func (p *Provider) ID() string {
	return providerID
}

func (p *Provider) Send(
	ctx context.Context,
	req sms.SendRequest,
) (*sms.SendResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("mnotify send failed: provider client is required")
	}

	mnotifyReq := FromInternal(req)

	var mnotifyResp SendResponse
	if err := p.client.doRequest(
		ctx,
		http.MethodPost,
		"/api/sms/quick",
		mnotifyReq,
		&mnotifyResp,
	); err != nil {
		return nil, fmt.Errorf("mnotify send failed: %w", err)
	}

	internalResp, err := ToInternal(&mnotifyResp)
	if err != nil {
		return nil, fmt.Errorf("mnotify send failed: %w", err)
	}

	return internalResp, nil
}

// CheckStatus checks the campaign-level delivery report using the campaign ID
// returned by Send.
func (p *Provider) CheckStatus(
	ctx context.Context,
	campaignID string,
) (*sms.StatusResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("mnotify status check failed: provider client is required")
	}

	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("mnotify status check failed: campaign id is required")
	}

	var mnotifyResp CampaignStatusResponse
	path := "/api/campaign/" + url.PathEscape(campaignID)
	if err := p.client.doRequest(
		ctx,
		http.MethodGet,
		path,
		nil,
		&mnotifyResp,
	); err != nil {
		return nil, fmt.Errorf("mnotify status check failed: %w", err)
	}

	internalResp, err := CampaignStatusToInternal(campaignID, &mnotifyResp)
	if err != nil {
		return nil, fmt.Errorf("mnotify status check failed: %w", err)
	}

	return internalResp, nil
}
