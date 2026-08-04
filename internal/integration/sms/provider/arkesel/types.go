package arkesel

// SendRequest represents Arkesel SMS API V2's /api/v2/sms/send payload.
type SendRequest struct {
	Sender        string   `json:"sender"`
	Message       string   `json:"message"`
	Recipients    []string `json:"recipients"`
	CallbackURL   string   `json:"callback_url,omitempty"`
	ScheduledDate string   `json:"scheduled_date,omitempty"`
	UseCase       string   `json:"use_case,omitempty"`
	Sandbox       bool     `json:"sandbox,omitempty"`
}

// MessageData can represent either an accepted recipient/message pair or the
// "invalid numbers" object that Arkesel may include in the data array.
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
