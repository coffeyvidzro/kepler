package sender

import (
	"strings"

	platformsenderid "github.com/coffeyvidzro/dugble/server/internal/platform/senderid"
)

const (
	createType = 3
	statusType = 1
)

type CreateRequest = platformsenderid.CreateRequest

type createRequest struct {
	Type      int               `json:"type"`
	SenderIDs []senderIDRequest `json:"senderids"`
}

type senderIDRequest struct {
	SenderID string `json:"senderid"`
}

func newCreateRequest(request platformsenderid.CreateRequest) createRequest {
	request = request.Normalize()
	return createRequest{
		Type:      createType,
		SenderIDs: []senderIDRequest{{SenderID: request.SenderID}},
	}
}

type statusRequest struct {
	Type     int    `json:"type"`
	SenderID string `json:"senderid"`
}

func newStatusRequest(senderID string) statusRequest {
	return statusRequest{Type: statusType, SenderID: strings.TrimSpace(senderID)}
}
