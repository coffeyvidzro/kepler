package celcom

import (
	"context"
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

type Provider struct {
	client *Client
}

func NewProvider(client *Client) *Provider { return &Provider{client: client} }
func (p *Provider) ID() string             { return providerID }

func (p *Provider) Send(ctx context.Context, req sms.SendRequest) (*sms.SendResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("celcom send failed: provider client is required")
	}

	var response SendResponse
	if err := p.client.doRequest(
		ctx,
		"/api/services/sendsms/",
		FromInternal(req, p.client.PartnerID, p.client.APIKey),
		&response,
	); err != nil {
		return nil, fmt.Errorf("celcom send failed: %w", err)
	}

	internal, err := ToInternal(&response)
	if err != nil {
		return nil, fmt.Errorf("celcom send failed: %w", err)
	}
	return internal, nil
}

func (p *Provider) CheckStatus(ctx context.Context, messageID string) (*sms.StatusResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("celcom status check failed: provider client is required")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("celcom status check failed: message id is required")
	}

	var response DeliveryReportResponse
	if err := p.client.doRequest(
		ctx,
		"/api/services/getdlr/",
		DeliveryReportFromInternal(messageID, p.client.PartnerID, p.client.APIKey),
		&response,
	); err != nil {
		return nil, fmt.Errorf("celcom status check failed: %w", err)
	}

	internal, err := StatusToInternal(messageID, &response)
	if err != nil {
		return nil, fmt.Errorf("celcom status check failed: %w", err)
	}
	return internal, nil
}
