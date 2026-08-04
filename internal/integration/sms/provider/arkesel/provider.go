package arkesel

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

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

func (p *Provider) Send(ctx context.Context, req sms.SendRequest) (*sms.SendResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("arkesel send failed: provider client is nil")
	}

	sender := strings.TrimSpace(req.From)
	recipient := strings.TrimSpace(req.To)
	message := strings.TrimSpace(req.Message)

	if sender == "" {
		return nil, fmt.Errorf("arkesel send failed: sender is required")
	}
	if utf8.RuneCountInString(sender) > 11 {
		return nil, fmt.Errorf("arkesel send failed: sender must not exceed 11 characters")
	}
	if recipient == "" {
		return nil, fmt.Errorf("arkesel send failed: recipient is required")
	}
	if message == "" {
		return nil, fmt.Errorf("arkesel send failed: message is required")
	}

	arkeselReq := FromInternal(req)

	var arkeselResp SendResponse
	if err := p.client.doRequest(
		ctx,
		http.MethodPost,
		"/api/v2/sms/send",
		arkeselReq,
		&arkeselResp,
	); err != nil {
		return nil, fmt.Errorf("arkesel send failed: %w", err)
	}

	internalResp, err := ToInternal(&arkeselResp)
	if err != nil {
		return nil, fmt.Errorf("arkesel send failed: %w", err)
	}

	return internalResp, nil
}

func (p *Provider) CheckStatus(ctx context.Context, messageID string) (*sms.StatusResponse, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("arkesel status check failed: provider client is nil")
	}

	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("arkesel status check failed: message id is required")
	}

	var arkeselResp StatusResponse
	path := "/api/v2/sms/" + url.PathEscape(messageID)
	if err := p.client.doRequest(ctx, http.MethodGet, path, nil, &arkeselResp); err != nil {
		return nil, fmt.Errorf("arkesel status check failed: %w", err)
	}

	internalResp, err := StatusToInternal(&arkeselResp)
	if err != nil {
		return nil, fmt.Errorf("arkesel status check failed: %w", err)
	}

	return internalResp, nil
}
