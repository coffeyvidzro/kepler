package arkesel

import (
	"strings"

	platformsms "github.com/coffeyvidzro/dugble/server/internal/platform/sms"
)

type SendRequest struct {
	Sender        string   `json:"sender"`
	Message       string   `json:"message"`
	Recipients    []string `json:"recipients"`
	CallbackURL   string   `json:"callback_url,omitempty"`
	ScheduledDate string   `json:"scheduled_date,omitempty"`
	UseCase       string   `json:"use_case,omitempty"`
	Sandbox       bool     `json:"sandbox,omitempty"`
}

func newSendRequest(request platformsms.SendRequest) SendRequest {
	return SendRequest{
		Sender:     strings.TrimSpace(request.From),
		Message:    request.Message,
		Recipients: []string{strings.TrimSpace(request.To)},
	}
}
