package celcom

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

const providerID = "celcom"

func FromInternal(req sms.SendRequest, partnerID, apiKey string) *SendRequest {
	return &SendRequest{
		PartnerID: strings.TrimSpace(partnerID),
		APIKey:    strings.TrimSpace(apiKey),
		Mobile:    strings.TrimPrefix(strings.TrimSpace(req.To), "+"),
		Message:   req.Message,
		Shortcode: strings.TrimSpace(req.From),
		PassType:  "plain",
	}
}

func DeliveryReportFromInternal(messageID, partnerID, apiKey string) *DeliveryReportRequest {
	return &DeliveryReportRequest{
		PartnerID: strings.TrimSpace(partnerID),
		APIKey:    strings.TrimSpace(apiKey),
		MessageID: strings.TrimSpace(messageID),
	}
}

func ToInternal(resp *SendResponse) (*sms.SendResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("celcom send response is nil")
	}
	if len(resp.Responses) == 0 {
		return nil, fmt.Errorf("celcom send response contains no results")
	}

	result := resp.Responses[0]
	if result.ResponseCode != 200 {
		return nil, &APIError{Code: result.ResponseCode, Description: result.ResponseDescription}
	}
	messageID := strings.TrimSpace(result.MessageID)
	if messageID == "" {
		return nil, fmt.Errorf("celcom send response contains empty message id")
	}

	return &sms.SendResponse{
		ProviderID:    providerID,
		ProviderMsgID: messageID,
		Status:        sms.StatusSubmitted,
	}, nil
}

func StatusToInternal(messageID string, resp *DeliveryReportResponse) (*sms.StatusResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("celcom delivery report response is nil")
	}
	if resp.ResponseCode != 200 {
		if resp.ResponseCode == 1008 {
			return &sms.StatusResponse{
				ProviderID: providerID, ProviderMsgID: strings.TrimSpace(messageID), Status: sms.StatusUnknown,
			}, nil
		}
		return nil, &APIError{Code: resp.ResponseCode, Description: resp.ResponseDescription}
	}

	return &sms.StatusResponse{
		ProviderID:    providerID,
		ProviderMsgID: strings.TrimSpace(messageID),
		Status:        NormalizeStatus(resp.ResponseDescription),
	}, nil
}

func NormalizeStatus(value string) string {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "DELIVERED"), normalized == "DELIVRD":
		return sms.StatusDelivered
	case strings.Contains(normalized, "UNDELIVERED"), normalized == "UNDELIV":
		return sms.StatusUndelivered
	case strings.Contains(normalized, "REJECT"):
		return sms.StatusRejected
	case strings.Contains(normalized, "EXPIRED"):
		return sms.StatusExpired
	case strings.Contains(normalized, "FAILED"), strings.Contains(normalized, "ERROR"):
		return sms.StatusFailed
	case normalized == "SENT":
		return sms.StatusSent
	case strings.Contains(normalized, "SUBMIT"), strings.Contains(normalized, "QUEUE"), strings.Contains(normalized, "PENDING"), strings.Contains(normalized, "SUCCESS"):
		return sms.StatusSubmitted
	default:
		return sms.StatusUnknown
	}
}
