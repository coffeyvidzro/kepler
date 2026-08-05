package sender

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	createType       = 3
	statusType       = 1
	maxSenderIDRunes = 11
)

type CreateRequest struct {
	SenderID string
}

func (request CreateRequest) Normalize() CreateRequest {
	request.SenderID = strings.TrimSpace(request.SenderID)
	return request
}

func (request CreateRequest) Validate() error {
	request = request.Normalize()
	if request.SenderID == "" {
		return fmt.Errorf("sender ID is required")
	}
	if utf8.RuneCountInString(request.SenderID) > maxSenderIDRunes {
		return fmt.Errorf("sender ID must not exceed %d characters", maxSenderIDRunes)
	}
	return nil
}

type createRequest struct {
	Type      int               `json:"type"`
	SenderIDs []senderIDRequest `json:"senderids"`
}

type senderIDRequest struct {
	SenderID string `json:"senderid"`
}

func newCreateRequest(request CreateRequest) createRequest {
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
