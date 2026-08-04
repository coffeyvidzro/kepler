package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"
)

const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSubmitted  = "submitted"
	StatusDelivered  = "delivered"
	StatusDelayed    = "delayed"
	StatusBounced    = "bounced"
	StatusComplained = "complained"
	StatusRejected   = "rejected"
	StatusFailed     = "failed"
	StatusCanceled   = "canceled"
)

const (
	MessageTypeTransactional = "transactional"
	MessageTypeMarketing     = "marketing"
)

type Message struct {
	ID                string            `json:"id"`
	TeamID            string            `json:"team_id"`
	MessageType       string            `json:"message_type"`
	FromEmail         string            `json:"from_email"`
	FromName          *string           `json:"from_name,omitempty"`
	ReplyToEmail      *string           `json:"reply_to_email,omitempty"`
	To                []EmailAddress    `json:"to"`
	CC                []EmailAddress    `json:"cc,omitempty"`
	BCC               []EmailAddress    `json:"bcc,omitempty"`
	ReplyTo           []EmailAddress    `json:"reply_to,omitempty"`
	ToEmail           string            `json:"to_email"`
	ToName            *string           `json:"to_name,omitempty"`
	Subject           string            `json:"subject"`
	HTMLBody          *string           `json:"html_body,omitempty"`
	TextBody          *string           `json:"text_body,omitempty"`
	Status            string            `json:"status"`
	Provider          *string           `json:"provider,omitempty"`
	ProviderMessageID *string           `json:"provider_message_id,omitempty"`
	ErrorCode         *string           `json:"error_code,omitempty"`
	ErrorMessage      *string           `json:"error_message,omitempty"`
	Metadata          json.RawMessage   `json:"metadata"`
	Headers           map[string]string `json:"headers,omitempty"`
	Attachments       []Attachment      `json:"attachments,omitempty"`
	Tags              []Tag             `json:"tags,omitempty"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty"`
	QueuedAt          time.Time         `json:"queued_at"`
	ProcessingAt      *time.Time        `json:"processing_at,omitempty"`
	SubmittedAt       *time.Time        `json:"submitted_at,omitempty"`
	DeliveredAt       *time.Time        `json:"delivered_at,omitempty"`
	FailedAt          *time.Time        `json:"failed_at,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type SendRequest struct {
	Stream      string            `json:"stream,omitempty"`
	From        *EmailAddress     `json:"from,omitempty"`
	ReplyTo     EmailAddressList  `json:"reply_to,omitempty"`
	To          EmailAddressList  `json:"to"`
	CC          EmailAddressList  `json:"cc,omitempty"`
	BCC         EmailAddressList  `json:"bcc,omitempty"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html,omitempty"`
	Text        string            `json:"text,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Attachments []Attachment      `json:"attachments,omitempty"`
	Tags        []Tag             `json:"tags,omitempty"`
	ScheduledAt string            `json:"scheduled_at,omitempty"`
	Metadata    json.RawMessage   `json:"metadata,omitempty"`
}

type EmailAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// UnmarshalJSON accepts both Resend's "Name <email>" string form and Dugble's
// original {"email":"...","name":"..."} form.
func (a *EmailAddress) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		parsed, err := parseEmailAddress(value)
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	}
	type alias EmailAddress
	return json.Unmarshal(data, (*alias)(a))
}

type EmailAddressList []EmailAddress

func (list *EmailAddressList) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		return nil
	}
	if data[0] == '[' {
		return json.Unmarshal(data, (*[]EmailAddress)(list))
	}
	var address EmailAddress
	if err := json.Unmarshal(data, &address); err != nil {
		return err
	}
	*list = []EmailAddress{address}
	return nil
}

type Attachment struct {
	Content     string `json:"content,omitempty"`
	Filename    string `json:"filename,omitempty"`
	Path        string `json:"path,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func parseEmailAddress(value string) (EmailAddress, error) {
	parsed, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return EmailAddress{}, fmt.Errorf("invalid email address")
	}
	return EmailAddress{Email: parsed.Address, Name: parsed.Name}, nil
}

type BatchSendRequest struct {
	Messages []SendRequest `json:"messages"`
}

// UnmarshalJSON accepts Resend's top-level array while retaining compatibility
// with the original {"messages": [...]} Dugble payload.
func (request *BatchSendRequest) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		return json.Unmarshal(data, &request.Messages)
	}
	type alias BatchSendRequest
	return json.Unmarshal(data, (*alias)(request))
}

type SendResponse struct {
	ID string `json:"id"`
}

type MutationResponse struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}

type UpdateRequest struct {
	ScheduledAt string `json:"scheduled_at"`
}

func SendResponses(messages []Message) []SendResponse {
	responses := make([]SendResponse, len(messages))
	for index, message := range messages {
		responses[index] = SendResponse{ID: message.ID}
	}
	return responses
}

type RetrieveResponse struct {
	Object      string     `json:"object"`
	ID          string     `json:"id"`
	MessageID   *string    `json:"message_id"`
	Stream      string     `json:"stream"`
	To          []string   `json:"to"`
	From        string     `json:"from"`
	CreatedAt   time.Time  `json:"created_at"`
	Subject     string     `json:"subject"`
	HTML        *string    `json:"html"`
	Text        *string    `json:"text"`
	BCC         []string   `json:"bcc"`
	CC          []string   `json:"cc"`
	ReplyTo     []string   `json:"reply_to"`
	LastEvent   string     `json:"last_event"`
	ScheduledAt *time.Time `json:"scheduled_at"`
	Tags        []Tag      `json:"tags"`
}

func (m Message) RetrieveResponse() RetrieveResponse {
	to := addressStrings(m.To)
	if len(to) == 0 && m.ToEmail != "" {
		to = addressStrings([]EmailAddress{{Email: m.ToEmail, Name: pointerValue(m.ToName)}})
	}
	replyTo := addressStrings(m.ReplyTo)
	if len(replyTo) == 0 && m.ReplyToEmail != nil {
		replyTo = []string{*m.ReplyToEmail}
	}
	return RetrieveResponse{
		Object: "email", ID: m.ID, MessageID: m.ProviderMessageID, Stream: m.MessageType,
		To: to, From: formatEmailAddress(EmailAddress{Email: m.FromEmail, Name: pointerValue(m.FromName)}),
		CreatedAt: m.CreatedAt, Subject: m.Subject, HTML: m.HTMLBody, Text: m.TextBody,
		BCC: addressStrings(m.BCC), CC: addressStrings(m.CC), ReplyTo: replyTo,
		LastEvent: m.Status, ScheduledAt: m.ScheduledAt, Tags: nonNilTags(m.Tags),
	}
}

func addressStrings(addresses []EmailAddress) []string {
	result := make([]string, len(addresses))
	for index, address := range addresses {
		result[index] = formatEmailAddress(address)
	}
	return result
}

func formatEmailAddress(address EmailAddress) string {
	email := strings.TrimSpace(address.Email)
	name := strings.TrimSpace(address.Name)
	if name == "" {
		return email
	}
	if isSimpleDisplayName(name) {
		return name + " <" + email + ">"
	}
	return (&mail.Address{Name: name, Address: email}).String()
}

func isSimpleDisplayName(name string) bool {
	for _, character := range name {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != ' ' && !strings.ContainsRune("._-", character) {
			return false
		}
	}
	return true
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nonNilTags(tags []Tag) []Tag {
	if tags == nil {
		return []Tag{}
	}
	return tags
}

type MessageSummary struct {
	ID          string     `json:"id"`
	ToEmail     string     `json:"to_email"`
	ToName      *string    `json:"to_name,omitempty"`
	Subject     string     `json:"subject"`
	Status      string     `json:"status"`
	Provider    *string    `json:"provider,omitempty"`
	QueuedAt    time.Time  `json:"queued_at"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (m Message) Summary() MessageSummary {
	return MessageSummary{ID: m.ID, ToEmail: m.ToEmail, ToName: m.ToName, Subject: m.Subject,
		Status: m.Status, Provider: m.Provider, QueuedAt: m.QueuedAt, SubmittedAt: m.SubmittedAt,
		DeliveredAt: m.DeliveredAt, CreatedAt: m.CreatedAt}
}

func Summaries(messages []Message) []MessageSummary {
	result := make([]MessageSummary, len(messages))
	for index, message := range messages {
		result[index] = message.Summary()
	}
	return result
}

type ListRequest struct{ Limit, Offset int32 }
