package sender

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

const ProviderID = platformsenderid.ProviderLeamout

var ErrSenderIDNotFound = errors.New("leamout sender ID was not found")

type Config struct {
	StatusSequence []string
}

func DefaultConfig() Config {
	return Config{StatusSequence: []string{
		platformsenderid.StatusPending,
		platformsenderid.StatusApproved,
	}}
}

type registrationState struct {
	senderID   string
	nextStatus int
}

type Provider struct {
	mu            sync.Mutex
	statuses      []string
	registrations map[string]*registrationState
}

var _ platformsenderid.Provider = (*Provider)(nil)

func NewProvider() *Provider {
	provider, _ := NewProviderWithConfig(DefaultConfig())
	return provider
}

func NewProviderWithConfig(config Config) (*Provider, error) {
	statuses, err := normalizeStatuses(config.StatusSequence)
	if err != nil {
		return nil, err
	}
	return &Provider{
		statuses:      statuses,
		registrations: make(map[string]*registrationState),
	}, nil
}

func (provider *Provider) ID() string { return ProviderID }

func (provider *Provider) Create(ctx context.Context, request platformsenderid.CreateRequest) (*platformsenderid.CreateResponse, error) {
	if provider == nil {
		return nil, errors.New("leamout sender ID provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("leamout sender ID context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}

	state := &registrationState{senderID: request.SenderID}
	status := provider.statuses[0]
	if len(provider.statuses) > 1 {
		state.nextStatus = 1
	}

	provider.mu.Lock()
	provider.registrations[strings.ToLower(request.SenderID)] = state
	provider.mu.Unlock()

	return &platformsenderid.CreateResponse{
		ProviderID: ProviderID,
		SenderID:   request.SenderID,
		Status:     status,
	}, nil
}

func (provider *Provider) CheckStatus(ctx context.Context, senderID string) (*platformsenderid.StatusResponse, error) {
	if provider == nil {
		return nil, errors.New("leamout sender ID provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("leamout sender ID context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	senderID = platformsenderid.NormalizeName(senderID)
	if err := platformsenderid.ValidateName(senderID); err != nil {
		return nil, err
	}

	provider.mu.Lock()
	state, exists := provider.registrations[strings.ToLower(senderID)]
	if !exists {
		provider.mu.Unlock()
		return nil, ErrSenderIDNotFound
	}
	index := state.nextStatus
	if index >= len(provider.statuses) {
		index = len(provider.statuses) - 1
	}
	status := provider.statuses[index]
	if state.nextStatus < len(provider.statuses)-1 {
		state.nextStatus++
	}
	registeredSenderID := state.senderID
	provider.mu.Unlock()

	return &platformsenderid.StatusResponse{
		ProviderID:     ProviderID,
		SenderID:       registeredSenderID,
		Status:         status,
		ProviderStatus: strings.ToUpper(ProviderID + "_" + status),
		Whitelisted:    status == platformsenderid.StatusApproved,
	}, nil
}

func normalizeStatuses(statuses []string) ([]string, error) {
	if len(statuses) == 0 {
		statuses = DefaultConfig().StatusSequence
	}
	normalized := make([]string, len(statuses))
	for index, status := range statuses {
		status = strings.ToLower(strings.TrimSpace(status))
		switch status {
		case platformsenderid.StatusPending, platformsenderid.StatusApproved, platformsenderid.StatusRejected:
			normalized[index] = status
		default:
			return nil, fmt.Errorf("leamout sender ID status sequence contains invalid status %q", status)
		}
	}
	return normalized, nil
}
