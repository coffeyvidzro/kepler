package sender

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusUnknown  = "unknown"
)

type CreateResponse struct {
	ProviderID string
	SenderID   string
	Status     string
}

type createResponse = moolre.Envelope[json.RawMessage]

func mapCreateResponse(senderID string, response *createResponse) (*CreateResponse, error) {
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
	return &CreateResponse{
		ProviderID: ProviderID,
		SenderID:   strings.TrimSpace(senderID),
		Status:     StatusPending,
	}, nil
}
