package mnotify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type ResponseCode string

func (code *ResponseCode) UnmarshalJSON(data []byte) error {
	if code == nil {
		return fmt.Errorf("%w: response code target is nil", ErrInvalidResponse)
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*code = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*code = ResponseCode(strings.TrimSpace(value))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("mNotify response code must be a string or number: %w", err)
	}
	*code = ResponseCode(number.String())
	return nil
}

func (code ResponseCode) String() string { return strings.TrimSpace(string(code)) }

type SendResponse struct {
	Status  string       `json:"status"`
	Code    ResponseCode `json:"code"`
	Message string       `json:"message"`
	Summary struct {
		ID            string   `json:"_id"`
		Type          string   `json:"type"`
		TotalSent     int      `json:"total_sent"`
		Contacts      int      `json:"contacts"`
		TotalRejected int      `json:"total_rejected"`
		NumbersSent   []string `json:"numbers_sent"`
		CreditUsed    int      `json:"credit_used"`
		CreditLeft    int      `json:"credit_left"`
	} `json:"summary"`
}

type CampaignStatusResponse struct {
	Status string         `json:"status"`
	Report []StatusReport `json:"report"`
}

type StatusReport struct {
	ID         int64  `json:"_id"`
	Recipient  string `json:"recipient"`
	Message    string `json:"message"`
	Sender     string `json:"sender"`
	Status     string `json:"status"`
	DateSent   string `json:"date_sent"`
	CampaignID string `json:"campaign_id"`
	Retries    int    `json:"retries"`
}

func mapSendResponse(response *SendResponse) (*platformsms.SendResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: send response is nil", ErrInvalidResponse)
	}
	if !success(response.Status) || response.Code.String() != "2000" {
		return nil, &APIError{Status: response.Status, Code: response.Code, Message: response.Message, Definitive: true}
	}
	campaignID := strings.TrimSpace(response.Summary.ID)
	if campaignID == "" {
		return nil, fmt.Errorf("%w: send response contains no campaign ID", ErrInvalidResponse)
	}
	if response.Summary.TotalSent <= 0 {
		return nil, &APIError{
			Status:     response.Status,
			Code:       response.Code,
			Message:    fmt.Sprintf("accepted no recipients: contacts %d rejected %d", response.Summary.Contacts, response.Summary.TotalRejected),
			Definitive: true,
		}
	}
	return &platformsms.SendResponse{ProviderID: ProviderID, ProviderMsgID: campaignID, Status: platformsms.StatusSubmitted}, nil
}

func mapCampaignStatusResponse(campaignID string, response *CampaignStatusResponse) (*platformsms.StatusResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: campaign status response is nil", ErrInvalidResponse)
	}
	if !success(response.Status) {
		return nil, &APIError{Status: response.Status, Message: "campaign status request failed", Definitive: true}
	}
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return nil, fmt.Errorf("mNotify campaign ID is required")
	}
	if len(response.Report) == 0 {
		return &platformsms.StatusResponse{ProviderID: ProviderID, ProviderMsgID: campaignID, Status: platformsms.StatusUnknown}, nil
	}
	report := response.Report[0]
	if report.CampaignID != "" && !strings.EqualFold(strings.TrimSpace(report.CampaignID), campaignID) {
		return nil, fmt.Errorf("%w: campaign ID %q does not match %q", ErrInvalidResponse, report.CampaignID, campaignID)
	}
	return &platformsms.StatusResponse{ProviderID: ProviderID, ProviderMsgID: campaignID, Status: normalizeStatus(report.Status)}, nil
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

func success(status string) bool { return strings.EqualFold(strings.TrimSpace(status), "success") }
