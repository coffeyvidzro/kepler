package sms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/runnage"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const ProviderID = "runnage"

type SendMode string

const (
	SendModeAccepted  SendMode = "accepted"
	SendModeRejected  SendMode = "rejected"
	SendModeUncertain SendMode = "uncertain"
)

type Config struct {
	SendMode       SendMode
	StatusSequence []string
}

func DefaultConfig() Config {
	return Config{
		SendMode: SendModeAccepted,
		StatusSequence: []string{
			platformsms.StatusSent,
			platformsms.StatusUndelivered,
		},
	}
}

type messageState struct {
	nextStatus int
}

type Provider struct {
	mu       sync.Mutex
	nextID   uint64
	mode     SendMode
	statuses []string
	messages map[string]*messageState
}

var _ platformsms.Provider = (*Provider)(nil)

func NewProvider() *Provider {
	provider, _ := NewProviderWithConfig(DefaultConfig())
	return provider
}

func NewProviderWithConfig(config Config) (*Provider, error) {
	if config.SendMode == "" {
		config.SendMode = SendModeAccepted
	}
	switch config.SendMode {
	case SendModeAccepted, SendModeRejected, SendModeUncertain:
	default:
		return nil, fmt.Errorf("runnage SMS send mode %q is invalid", config.SendMode)
	}
	statuses, err := normalizeStatuses(config.StatusSequence)
	if err != nil {
		return nil, err
	}
	return &Provider{
		mode:     config.SendMode,
		statuses: statuses,
		messages: make(map[string]*messageState),
	}, nil
}

func (provider *Provider) ID() string { return ProviderID }

func (provider *Provider) Send(ctx context.Context, request platformsms.SendRequest) (*platformsms.SendResponse, error) {
	if provider == nil {
		return nil, errors.New("runnage SMS provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("runnage SMS context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request = request.Normalize()
	if err := request.Validate(); err != nil {
		return nil, err
	}

	switch provider.mode {
	case SendModeRejected:
		return nil, &runnage.Error{
			Code:       "sms_rejected",
			Message:    "runnage rejected the SMS before acceptance",
			Definitive: true,
		}
	case SendModeUncertain:
		return nil, &runnage.Error{
			Code:    "sms_outcome_unknown",
			Message: "runnage lost the connection after submission",
		}
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
		return nil, errors.New("runnage SMS provider is unavailable")
	}
	if ctx == nil {
		return nil, errors.New("runnage SMS context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, errors.New("runnage SMS message ID is required")
	}

	provider.mu.Lock()
	state, exists := provider.messages[messageID]
	if !exists {
		provider.mu.Unlock()
		return nil, &runnage.Error{
			Code:       "sms_not_found",
			Message:    "runnage does not recognize the SMS message ID",
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
			return nil, fmt.Errorf("runnage SMS status sequence contains invalid status %q", status)
		}
		normalized[index] = status
	}
	return normalized, nil
}
