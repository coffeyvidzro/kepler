package sender

import (
	"context"
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

const (
	ProviderID = platformsenderid.ProviderMoolre
	createPath = "/open/sms/query"
	statusPath = "/open/sms/status"
)

type Provider struct{ client *moolre.Client }

var _ platformsenderid.Provider = (*Provider)(nil)

func NewProvider(client *moolre.Client) *Provider { return &Provider{client: client} }
func (provider *Provider) ID() string             { return ProviderID }

func (provider *Provider) Create(ctx context.Context, request CreateRequest) (*CreateResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("moolre sender ID creation failed: %w", moolre.ErrClientUnavailable)
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("moolre sender ID creation failed: %w", err)
	}

	var response createResponse
	if err := provider.client.Post(ctx, createPath, newCreateRequest(request), &response); err != nil {
		return nil, fmt.Errorf("moolre sender ID creation failed: %w", err)
	}
	mapped, err := mapCreateResponse(request.SenderID, &response)
	if err != nil {
		return nil, fmt.Errorf("moolre sender ID creation failed: %w", err)
	}
	return mapped, nil
}

func (provider *Provider) CheckStatus(ctx context.Context, senderID string) (*StatusResponse, error) {
	if provider == nil || provider.client == nil {
		return nil, fmt.Errorf("moolre sender ID status check failed: %w", moolre.ErrClientUnavailable)
	}
	senderID = strings.TrimSpace(senderID)
	if err := platformsenderid.ValidateName(senderID); err != nil {
		return nil, fmt.Errorf("moolre sender ID status check failed: %w", err)
	}

	var response statusResponse
	if err := provider.client.Post(ctx, statusPath, newStatusRequest(senderID), &response); err != nil {
		return nil, fmt.Errorf("moolre sender ID status check failed: %w", err)
	}
	mapped, err := mapStatusResponse(senderID, &response)
	if err != nil {
		return nil, fmt.Errorf("moolre sender ID status check failed: %w", err)
	}
	return mapped, nil
}
