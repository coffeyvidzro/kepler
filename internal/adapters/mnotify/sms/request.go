package sms

import (
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type sendRequest struct {
	Recipient    []string `json:"recipient"`
	Sender       string   `json:"sender"`
	Message      string   `json:"message"`
	IsSchedule   bool     `json:"is_schedule"`
	ScheduleDate string   `json:"schedule_date"`
}

func newSendRequest(request platformsms.SendRequest) sendRequest {
	return sendRequest{
		Recipient:    []string{strings.TrimPrefix(strings.TrimSpace(request.To), "+")},
		Sender:       strings.TrimSpace(request.From),
		Message:      request.Message,
		IsSchedule:   false,
		ScheduleDate: "",
	}
}
