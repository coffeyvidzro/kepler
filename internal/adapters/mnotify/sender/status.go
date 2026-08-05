package sender

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

type StatusResponse = platformsenderid.StatusResponse

type statusResponse struct {
	mnotify.Response
	Summary senderSummary `json:"summary"`
}

func mapStatusResponse(senderID string, response *statusResponse) (*platformsenderid.StatusResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: Sender ID status response is nil", mnotify.ErrInvalidResponse)
	}
	if !response.Successful() {
		return nil, &mnotify.APIError{
			Status:     strings.TrimSpace(response.Status),
			Code:       response.Code,
			Message:    strings.TrimSpace(response.Message),
			Definitive: true,
		}
	}

	expected := platformsenderid.NormalizeName(senderID)
	actual := response.Summary.senderID()
	if actual != "" && !strings.EqualFold(actual, expected) {
		return nil, fmt.Errorf("%w: Sender ID %q does not match %q", mnotify.ErrInvalidResponse, actual, expected)
	}
	providerStatus := strings.TrimSpace(response.Summary.Status)
	if providerStatus == "" {
		return nil, fmt.Errorf("%w: Sender ID status response contains no approval status", mnotify.ErrInvalidResponse)
	}
	return &platformsenderid.StatusResponse{
		ProviderID:     ProviderID,
		SenderID:       expected,
		Status:         normalizeStatus(providerStatus),
		ProviderStatus: providerStatus,
		Whitelisted:    false,
	}, nil
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved", "approve":
		return platformsenderid.StatusApproved
	case "rejected", "reject", "declined", "decline", "denied", "deny":
		return platformsenderid.StatusRejected
	case "pending", "on hold", "on-hold", "processing", "submitted":
		return platformsenderid.StatusPending
	default:
		return platformsenderid.StatusUnknown
	}
}
