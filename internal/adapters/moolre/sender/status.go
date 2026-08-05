package sender

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

type StatusResponse = platformsenderid.StatusResponse

type statusData struct {
	SenderID    string `json:"senderid"`
	Approval    string `json:"approval"`
	Whitelisted bool   `json:"whitelisted"`
}

type statusResponse = moolre.Envelope[statusData]

func mapStatusResponse(senderID string, response *statusResponse) (*platformsenderid.StatusResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: Sender ID status response is nil", moolre.ErrInvalidResponse)
	}
	if !response.Successful() {
		return nil, &moolre.APIError{
			Status:     response.Status,
			Code:       strings.TrimSpace(response.Code),
			Message:    response.Message.String(),
			Definitive: true,
		}
	}
	if !strings.EqualFold(strings.TrimSpace(response.Code), "ASMQ01") {
		return nil, fmt.Errorf("%w: successful Sender ID status response has code %q", moolre.ErrInvalidResponse, response.Code)
	}

	expected := strings.TrimSpace(senderID)
	actual := strings.TrimSpace(response.Data.SenderID)
	if actual == "" {
		return nil, fmt.Errorf("%w: Sender ID status response contains no Sender ID", moolre.ErrInvalidResponse)
	}
	if !strings.EqualFold(actual, expected) {
		return nil, fmt.Errorf("%w: Sender ID %q does not match %q", moolre.ErrInvalidResponse, actual, expected)
	}
	providerStatus := strings.TrimSpace(response.Data.Approval)
	return &platformsenderid.StatusResponse{
		ProviderID:     ProviderID,
		SenderID:       expected,
		Status:         normalizeStatus(providerStatus),
		ProviderStatus: providerStatus,
		Whitelisted:    response.Data.Whitelisted,
	}, nil
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return platformsenderid.StatusPending
	case "approved":
		return platformsenderid.StatusApproved
	case "rejected":
		return platformsenderid.StatusRejected
	default:
		return platformsenderid.StatusUnknown
	}
}
