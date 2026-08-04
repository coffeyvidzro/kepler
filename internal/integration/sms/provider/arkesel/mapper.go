package arkesel

import (
	"fmt"
	"strings"

	"github.com/coffeyvidzro/dugble/server/internal/integration/sms"
)

const providerID = "arkesel"

func FromInternal(req sms.SendRequest) *SendRequest {
	return &SendRequest{
		Sender:     strings.TrimSpace(req.From),
		Message:    req.Message,
		Recipients: []string{strings.TrimSpace(req.To)},
	}
}

func ToInternal(resp *SendResponse) (*sms.SendResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("arkesel send response is nil")
	}
	if !isSuccess(resp.Status) {
		return nil, fmt.Errorf("arkesel send response status is %q", resp.Status)
	}

	var invalidNumbers []string
	for _, item := range resp.Data {
		if len(item.InvalidNumbers) > 0 {
			invalidNumbers = append(invalidNumbers, item.InvalidNumbers...)
		}

		if strings.TrimSpace(item.ID) == "" {
			continue
		}

		return &sms.SendResponse{
			ProviderID:    providerID,
			ProviderMsgID: item.ID,
			Status:        "submitted",
		}, nil
	}

	if len(invalidNumbers) > 0 {
		return nil, fmt.Errorf(
			"arkesel rejected recipient(s): %s",
			strings.Join(invalidNumbers, ", "),
		)
	}

	return nil, fmt.Errorf("arkesel send response contains no message id")
}

func StatusToInternal(resp *StatusResponse) (*sms.StatusResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("arkesel status response is nil")
	}
	if !isSuccess(resp.Status) {
		return nil, fmt.Errorf("arkesel status response status is %q", resp.Status)
	}
	if strings.TrimSpace(resp.Data.ID) == "" {
		return nil, fmt.Errorf("arkesel status response contains empty message id")
	}

	return &sms.StatusResponse{
		ProviderID:    providerID,
		ProviderMsgID: resp.Data.ID,
		Status:        NormalizeStatus(resp.Data.Status),
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

func NormalizeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "OK", "ACCEPTED", "QUEUED", "SUBMITTED", "PENDING":
		return "submitted"
	case "SENT":
		return "sent"
	case "DELIVERED", "DELIVRD":
		return "delivered"
	case "UNDELIVERED", "UNDELIV":
		return "undelivered"
	case "REJECTED":
		return "rejected"
	case "FAILED", "FAILURE", "ERROR":
		return "failed"
	case "EXPIRED":
		return "expired"
	default:
		return "unknown"
	}
}
