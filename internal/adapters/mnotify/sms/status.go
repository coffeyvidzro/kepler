package sms

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/adapters/mnotify"
	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type statusReport struct {
	ID         mnotify.Identifier `json:"_id"`
	Recipient  string             `json:"recipient"`
	Message    string             `json:"message"`
	Sender     string             `json:"sender"`
	Status     string             `json:"status"`
	DateSent   string             `json:"date_sent"`
	CampaignID string             `json:"campaign_id"`
	Retries    int                `json:"retries"`
}

type statusResponse struct {
	mnotify.Response
	Report []statusReport `json:"report"`
}

func mapStatusResponse(campaignID string, response *statusResponse) (*platformsms.StatusResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: campaign status response is nil", mnotify.ErrInvalidResponse)
	}
	if !response.Successful() {
		return nil, &mnotify.APIError{
			Status:     strings.TrimSpace(response.Status),
			Code:       response.Code,
			Message:    strings.TrimSpace(response.Message),
			Definitive: true,
		}
	}
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("mNotify campaign ID is required")
	}
	if len(response.Report) == 0 {
		return &platformsms.StatusResponse{
			ProviderID:    ProviderID,
			ProviderMsgID: campaignID,
			Status:        platformsms.StatusUnknown,
		}, nil
	}

	report := response.Report[0]
	if report.CampaignID != "" && !strings.EqualFold(strings.TrimSpace(report.CampaignID), campaignID) {
		return nil, fmt.Errorf("%w: campaign ID %q does not match %q", mnotify.ErrInvalidResponse, report.CampaignID, campaignID)
	}
	providerStatus := strings.TrimSpace(report.Status)
	return &platformsms.StatusResponse{
		ProviderID:     ProviderID,
		ProviderMsgID:  campaignID,
		Status:         normalizeStatus(providerStatus),
		ProviderStatus: providerStatus,
	}, nil
}

func normalizeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "2000", "QUEUED", "PENDING", "SUBMITTED":
		return platformsms.StatusSubmitted
	case "SENT":
		return platformsms.StatusSent
	case "DELIVERED", "DELIVRD":
		return platformsms.StatusDelivered
	case "UNDELIVERED", "UNDELIV":
		return platformsms.StatusUndelivered
	case "REJECTED", "REJECTD":
		return platformsms.StatusRejected
	case "FAILED", "FAILURE", "ERROR":
		return platformsms.StatusFailed
	case "EXPIRED":
		return platformsms.StatusExpired
	default:
		return platformsms.StatusUnknown
	}
}
