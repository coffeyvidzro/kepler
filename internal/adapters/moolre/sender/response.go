package sender

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

const (
	StatusPending  = platformsenderid.StatusPending
	StatusApproved = platformsenderid.StatusApproved
	StatusRejected = platformsenderid.StatusRejected
	StatusUnknown  = platformsenderid.StatusUnknown
)

type CreateResponse = platformsenderid.CreateResponse

type createResponse = moolre.Envelope[json.RawMessage]

func mapCreateResponse(senderID string, response *createResponse) (*platformsenderid.CreateResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: Sender ID creation response is nil", moolre.ErrInvalidResponse)
	}
	if !response.Successful() {
		return nil, &moolre.APIError{
			Status:     response.Status,
			Code:       strings.TrimSpace(response.Code),
			Message:    response.Message.String(),
			Definitive: true,
		}
	}
	if !strings.EqualFold(strings.TrimSpace(response.Code), "ASMQ12") {
		return nil, fmt.Errorf("%w: successful Sender ID creation response has code %q", moolre.ErrInvalidResponse, response.Code)
	}
	return &platformsenderid.CreateResponse{
		ProviderID: ProviderID,
		SenderID:   strings.TrimSpace(senderID),
		Status:     platformsenderid.StatusPending,
	}, nil
}
