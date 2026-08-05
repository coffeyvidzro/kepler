package sms

import (
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

const (
	sendType   = 1
	statusType = 5
)

type sendRequest struct {
	Type     int              `json:"type"`
	SenderID string           `json:"senderid"`
	Messages []messageRequest `json:"messages"`
}

type messageRequest struct {
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
	Reference string `json:"ref"`
}

func newSendRequest(request platformsms.SendRequest) sendRequest {
	request = request.Normalize()
	return sendRequest{
		Type:     sendType,
		SenderID: request.From,
		Messages: []messageRequest{{
			Recipient: strings.TrimPrefix(request.To, "+"),
			Message:   request.Message,
			Reference: request.Reference,
		}},
	}
}

type statusRequest struct {
	Type       int      `json:"type"`
	References []string `json:"ref"`
}

func newStatusRequest(references []string) statusRequest {
	return statusRequest{Type: statusType, References: references}
}
