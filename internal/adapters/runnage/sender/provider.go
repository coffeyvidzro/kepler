package sender

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/runnage"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

const ProviderID = platformsenderid.ProviderRunnage

type CreateMode string

const (
	CreateModeAccepted  CreateMode = "accepted"
	CreateModeRejected  CreateMode = "rejected"
	CreateModeUncertain CreateMode = "uncertain"
)

type Config struct {
	CreateMode     CreateMode
	StatusSequence []string
}

func DefaultConfig() Config {
	return Config{
		CreateMode: CreateModeAccepted,
		StatusSequence: []string{
			platformsenderid.StatusPending,
			platformsenderid.StatusRejected,
		},
	}
}

type registrationState struct {
	senderID   string
	nextStatus int
}

type Provider struct {
	mu            sync.Mutex
	mode          CreateMode
	statuses      []string
	registrations map[string]*registrationState
}

var _ platformsenderid.Provider = (*Provider)(nil)

func NewProvider() *Provider {
	provider, _ := NewProviderWithConfig(DefaultConfig())
	return provider
}

func NewProviderWithConfig(config Config) (*Provider, error) {
	if config.CreateMode == "" {
		config.CreateMode = CreateModeAccepted
	}
	switch config.CreateMode {
	case CreateModeAccepted, CreateModeRejected, CreateModeUncertain:
	default:
		return nil, fmt.Errorf("runnage sender ID create mode %q is invalid", config.CreateMode)
	}
	statuses, err := normalizeStatuses(config.StatusSequence)
	if err != nil {
		return nil, err
	}
	return &Provider{
		mode:          config.CreateMode,
		statuses:      statuses,
		registrations: make(map[string]*registrationState),
	}, nil
}

func (provider *Provider) ID() string { return ProviderID }

func (provider *Provider) Create(ctx context.Context, request platformsenderid.CreateRequest) (*platformsenderid.CreateResponse, error) {
	if provider == nil {
		return nil, errors.New("runnage sender ID provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("runnage sender ID context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}

	switch provider.mode {
	case CreateModeRejected:
		return &platformsenderid.CreateResponse{
			ProviderID: ProviderID,
			SenderID:   request.SenderID,
			Status:     platformsenderid.StatusRejected,
		}, nil
	case CreateModeUncertain:
		return nil, &runnage.Error{
			Code:    "sender_submission_unknown",
			Message: "runnage lost the connection after sender ID submission",
		}
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
		return nil, errors.New("runnage sender ID provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("runnage sender ID context is required")
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
		return nil, &runnage.Error{
			Code:       "sender_not_found",
			Message:    "runnage does not recognize the sender ID",
			Definitive: true,
		}
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
			return nil, fmt.Errorf("runnage sender ID status sequence contains invalid status %q", status)
		}
	}
	return normalized, nil
}
