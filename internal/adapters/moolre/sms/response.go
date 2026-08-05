package sms

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/moolre"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type sendResponse = moolre.Envelope[json.RawMessage]

func mapSendResponse(reference string, response *sendResponse) (*platformsms.SendResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: send response is nil", moolre.ErrInvalidResponse)
	}
	if !response.Successful() {
		return nil, &moolre.APIError{
			Status:     response.Status,
			Code:       strings.TrimSpace(response.Code),
			Message:    response.Message.String(),
			Definitive: true,
		}
	}
	if !strings.EqualFold(strings.TrimSpace(response.Code), "SMS01") {
		return nil, fmt.Errorf("%w: successful send response has code %q", moolre.ErrInvalidResponse, response.Code)
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, fmt.Errorf("%w: SMS reference is empty", moolre.ErrInvalidResponse)
	}
	return &platformsms.SendResponse{
		ProviderID:    ProviderID,
		ProviderMsgID: reference,
		Status:        platformsms.StatusSubmitted,
	}, nil
}
