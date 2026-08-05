package sender

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

type CreateResponse = platformsenderid.CreateResponse

type senderSummary struct {
	SenderName       string `json:"sender_name"`
	LegacySenderName string `json:"sender name"`
	Purpose          string `json:"purpose"`
	Status           string `json:"status"`
}

func (summary senderSummary) senderID() string {
	if value := strings.TrimSpace(summary.SenderName); value != "" {
		return value
	}
	return strings.TrimSpace(summary.LegacySenderName)
}

type createResponse struct {
	mnotify.Response
	Summary senderSummary `json:"summary"`
}

func mapCreateResponse(senderID string, response *createResponse) (*platformsenderid.CreateResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: Sender ID creation response is nil", mnotify.ErrInvalidResponse)
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
		providerStatus = "Pending"
	}
	return &platformsenderid.CreateResponse{
		ProviderID: ProviderID,
		SenderID:   expected,
		Status:     normalizeStatus(providerStatus),
	}, nil
}
