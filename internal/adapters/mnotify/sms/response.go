package sms

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type sendSummary struct {
	ID            mnotify.Identifier `json:"_id"`
	Type          string             `json:"type"`
	TotalSent     int                `json:"total_sent"`
	Contacts      int                `json:"contacts"`
	TotalRejected int                `json:"total_rejected"`
	NumbersSent   []string           `json:"numbers_sent"`
	CreditUsed    int                `json:"credit_used"`
	CreditLeft    int                `json:"credit_left"`
}

type sendResponse struct {
	mnotify.Response
	Summary sendSummary `json:"summary"`
}

func mapSendResponse(response *sendResponse) (*platformsms.SendResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: send response is nil", mnotify.ErrInvalidResponse)
	}
	if !response.Successful() || response.Code.String() != "2000" {
		return nil, &mnotify.APIError{
			Status:     strings.TrimSpace(response.Status),
			Code:       response.Code,
			Message:    strings.TrimSpace(response.Message),
			Definitive: true,
		}
	}
	campaignID := response.Summary.ID.String()
	if campaignID == "" {
		return nil, fmt.Errorf("%w: send response contains no campaign ID", mnotify.ErrInvalidResponse)
	}
	if response.Summary.TotalSent <= 0 {
		return nil, &mnotify.APIError{
			Status:     strings.TrimSpace(response.Status),
			Code:       response.Code,
			Message:    fmt.Sprintf("accepted no recipients: contacts %d rejected %d", response.Summary.Contacts, response.Summary.TotalRejected),
			Definitive: true,
		}
	}
	return &platformsms.SendResponse{
		ProviderID:    ProviderID,
		ProviderMsgID: campaignID,
		Status:        platformsms.StatusSubmitted,
	}, nil
}
