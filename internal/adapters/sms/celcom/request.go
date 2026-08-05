package celcom

import (
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type SendRequest struct {
	PartnerID string `json:"partnerID"`
	APIKey    string `json:"apikey"`
	Mobile    string `json:"mobile"`
	Message   string `json:"message"`
	Shortcode string `json:"shortcode"`
	PassType  string `json:"pass_type"`
}

type DeliveryReportRequest struct {
	PartnerID string `json:"partnerID"`
	APIKey    string `json:"apikey"`
	MessageID string `json:"messageID"`
}

func newSendRequest(request platformsms.SendRequest, partnerID, apiKey string) SendRequest {
	return SendRequest{
		PartnerID: strings.TrimSpace(partnerID),
		APIKey:    strings.TrimSpace(apiKey),
		Mobile:    strings.TrimPrefix(strings.TrimSpace(request.To), "+"),
		Message:   request.Message,
		Shortcode: strings.TrimSpace(request.From),
		PassType:  "plain",
	}
}

func newDeliveryReportRequest(messageID, partnerID, apiKey string) DeliveryReportRequest {
	return DeliveryReportRequest{
		PartnerID: strings.TrimSpace(partnerID),
		APIKey:    strings.TrimSpace(apiKey),
		MessageID: strings.TrimSpace(messageID),
	}
}
