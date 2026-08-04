package mnotify

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

const providerID = "mnotify"

func FromInternal(req sms.SendRequest) *SendRequest {
	return &SendRequest{
		Recipient:    []string{strings.TrimPrefix(strings.TrimSpace(req.To), "+")},
		Sender:       strings.TrimSpace(req.From),
		Message:      req.Message,
		IsSchedule:   false,
		ScheduleDate: "",
	}
}

func ToInternal(resp *SendResponse) (*sms.SendResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("mnotify send response is nil")
	}

	if !success(resp.Status) || resp.Code.String() != "2000" {
		return nil, &APIError{
			Status:     resp.Status,
			Code:       resp.Code,
			Message:    resp.Message,
			Definitive: true,
		}
	}

	campaignID := strings.TrimSpace(resp.Summary.ID)
	if campaignID == "" {
		return nil, fmt.Errorf("mnotify send response contains empty campaign id")
	}
	if resp.Summary.TotalSent <= 0 {
		return nil, &APIError{
			Status:     resp.Status,
			Code:       resp.Code,
			Message:    fmt.Sprintf("accepted no recipients: contacts %d rejected %d", resp.Summary.Contacts, resp.Summary.TotalRejected),
			Definitive: true,
		}
	}

	return &sms.SendResponse{
		ProviderID:    providerID,
		ProviderMsgID: campaignID,
		Status:        sms.StatusSubmitted,
	}, nil
}

func CampaignStatusToInternal(
	campaignID string,
	resp *CampaignStatusResponse,
) (*sms.StatusResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("mnotify campaign status response is nil")
	}
	if !success(resp.Status) {
		return nil, &APIError{
			Status:     resp.Status,
			Message:    "campaign status request failed",
			Definitive: true,
		}
	}

	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("mnotify campaign id is required")
	}
	if len(resp.Report) == 0 {
		return &sms.StatusResponse{
			ProviderID:    providerID,
			ProviderMsgID: campaignID,
			Status:        sms.StatusUnknown,
		}, nil
	}

	// Dugble sends one recipient per provider request, so one report entry is
	// expected for the campaign. Multi-recipient campaigns require a different
	// internal status model rather than collapsing recipient-level outcomes.
	report := resp.Report[0]
	if report.CampaignID != "" && !strings.EqualFold(strings.TrimSpace(report.CampaignID), campaignID) {
		return nil, fmt.Errorf(
			"mnotify campaign status response has campaign id %q, expected %q",
			report.CampaignID,
			campaignID,
		)
	}

	return &sms.StatusResponse{
		ProviderID:    providerID,
		ProviderMsgID: campaignID,
		Status:        NormalizeStatus(report.Status),
	}, nil
}

func NormalizeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "2000", "QUEUED", "PENDING", "SUBMITTED":
		return sms.StatusSubmitted
	case "SENT":
		return sms.StatusSent
	case "DELIVERED", "DELIVRD":
		return sms.StatusDelivered
	case "UNDELIVERED", "UNDELIV":
		return sms.StatusUndelivered
	case "REJECTED", "REJECTD":
		return sms.StatusRejected
	case "FAILED", "FAILURE", "ERROR":
		return sms.StatusFailed
	case "EXPIRED":
		return sms.StatusExpired
	default:
		return sms.StatusUnknown
	}
}

func success(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "success")
}
