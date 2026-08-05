package arkesel

import (
	"fmt"
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type MessageData struct {
	Recipient      string   `json:"recipient,omitempty"`
	ID             string   `json:"id,omitempty"`
	InvalidNumbers []string `json:"invalid numbers,omitempty"`
}

type SendResponse struct {
	Status  string        `json:"status"`
	Message string        `json:"message,omitempty"`
	Data    []MessageData `json:"data"`
}

type StatusData struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Sender       string `json:"sender"`
	Recipient    string `json:"recipient"`
	Message      string `json:"message"`
	MessageCount int    `json:"message_count"`
	SentAtTime   string `json:"sent_at_time"`
}

type StatusResponse struct {
	Status  string     `json:"status"`
	Message string     `json:"message,omitempty"`
	Data    StatusData `json:"data"`
}

func mapSendResponse(response *SendResponse) (*platformsms.SendResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: send response is nil", ErrInvalidResponse)
	}
	if !isSuccess(response.Status) {
		return nil, fmt.Errorf("%w: send status is %q", ErrInvalidResponse, response.Status)
	}
	var invalidNumbers []string
	for _, item := range response.Data {
		if len(item.InvalidNumbers) > 0 {
			invalidNumbers = append(invalidNumbers, item.InvalidNumbers...)
		}
		if messageID := strings.TrimSpace(item.ID); messageID != "" {
			return &platformsms.SendResponse{
				ProviderID:    ProviderID,
				ProviderMsgID: messageID,
				Status:        platformsms.StatusSubmitted,
			}, nil
		}
	}
	if len(invalidNumbers) > 0 {
		return nil, &APIError{StatusCode: 422, Body: "rejected recipient(s): " + strings.Join(invalidNumbers, ", ")}
	}
	return nil, fmt.Errorf("%w: send response contains no message ID", ErrInvalidResponse)
}

func mapStatusResponse(response *StatusResponse) (*platformsms.StatusResponse, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: status response is nil", ErrInvalidResponse)
	}
	if !isSuccess(response.Status) {
		return nil, fmt.Errorf("%w: status response is %q", ErrInvalidResponse, response.Status)
	}
	messageID := strings.TrimSpace(response.Data.ID)
	if messageID == "" {
		return nil, fmt.Errorf("%w: status response contains no message ID", ErrInvalidResponse)
	}
	return &platformsms.StatusResponse{
		ProviderID:    ProviderID,
		ProviderMsgID: messageID,
		Status:        normalizeStatus(response.Data.Status),
	}, nil
}

func isSuccess(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "OK":
		return true
	default:
		return false
	}
}

func normalizeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "OK", "ACCEPTED", "QUEUED", "SUBMITTED", "PENDING":
		return platformsms.StatusSubmitted
	case "SENT":
		return platformsms.StatusSent
	case "DELIVERED", "DELIVRD":
		return platformsms.StatusDelivered
	case "UNDELIVERED", "UNDELIV":
		return platformsms.StatusUndelivered
	case "REJECTED":
		return platformsms.StatusRejected
	case "FAILED", "FAILURE", "ERROR":
		return platformsms.StatusFailed
	case "EXPIRED":
		return platformsms.StatusExpired
	default:
		return platformsms.StatusUnknown
	}
}
