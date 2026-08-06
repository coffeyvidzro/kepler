package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const ProviderID = "leamout"

var ErrMessageNotFound = errors.New("leamout SMS message was not found")

type Config struct {
	StatusSequence []string
}

func DefaultConfig() Config {
	return Config{StatusSequence: []string{
		platformsms.StatusSent,
		platformsms.StatusDelivered,
	}}
}

type messageState struct {
	nextStatus int
}

type Provider struct {
	mu       sync.Mutex
	nextID   uint64
	statuses []string
	messages map[string]*messageState
}

var _ platformsms.Provider = (*Provider)(nil)

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
		statuses: statuses,
		messages: make(map[string]*messageState),
	}, nil
}

func (provider *Provider) ID() string { return ProviderID }

func (provider *Provider) Send(ctx context.Context, request platformsms.SendRequest) (*platformsms.SendResponse, error) {
	if provider == nil {
		return nil, errors.New("leamout SMS provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("leamout SMS context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}

	provider.mu.Lock()
	provider.nextID++
	messageID := fmt.Sprintf("%s:sms:%06d", ProviderID, provider.nextID)
	provider.messages[messageID] = &messageState{}
	provider.mu.Unlock()

	return &platformsms.SendResponse{
		ProviderID:    ProviderID,
		ProviderMsgID: messageID,
		Status:        platformsms.StatusSubmitted,
	}, nil
}

func (provider *Provider) CheckStatus(ctx context.Context, messageID string) (*platformsms.StatusResponse, error) {
	if provider == nil {
		return nil, errors.New("leamout SMS provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("leamout SMS context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, errors.New("leamout SMS message ID is required")
	}

	provider.mu.Lock()
	state, exists := provider.messages[messageID]
	if !exists {
		provider.mu.Unlock()
		return nil, ErrMessageNotFound
	}
	index := state.nextStatus
	if index >= len(provider.statuses) {
		index = len(provider.statuses) - 1
	}
	status := provider.statuses[index]
	if state.nextStatus < len(provider.statuses)-1 {
		state.nextStatus++
	}
	provider.mu.Unlock()

	return &platformsms.StatusResponse{
		ProviderID:     ProviderID,
		ProviderMsgID:  messageID,
		Status:         status,
		ProviderStatus: strings.ToUpper(ProviderID + "_" + status),
	}, nil
}

func normalizeStatuses(statuses []string) ([]string, error) {
	if len(statuses) == 0 {
		statuses = DefaultConfig().StatusSequence
	}
	normalized := make([]string, len(statuses))
	for index, status := range statuses {
		status = strings.ToLower(strings.TrimSpace(status))
		if !platformsms.IsKnownStatus(status) || status == platformsms.StatusUnknown {
			return nil, fmt.Errorf("leamout SMS status sequence contains invalid status %q", status)
		}
		normalized[index] = status
	}
	return normalized, nil
}
