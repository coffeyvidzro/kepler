package sms

import (
	"bytes"
	"encoding/json"
	"time"
)

const (
	StatusQueued      = "queued"
	StatusProcessing  = "processing"
	StatusSubmitted   = "submitted"
	StatusSent        = "sent"
	StatusDelivered   = "delivered"
	StatusUndelivered = "undelivered"
	StatusRejected    = "rejected"
	StatusFailed      = "failed"
	StatusExpired     = "expired"
	StatusUnknown     = "unknown"
	StatusCanceled    = "canceled"
)

type Message struct {
	ID                 string          `json:"id"`
	TeamID             string          `json:"team_id"`
	SenderID           *string         `json:"sender_id,omitempty"`
	To                 string          `json:"to"`
	From               string          `json:"from"`
	Body               string          `json:"body"`
	Status             string          `json:"status"`
	ProviderID         *string         `json:"provider_id,omitempty"`
	ProviderMessageID  *string         `json:"provider_message_id,omitempty"`
	Segments           int32           `json:"segments"`
	ErrorMessage       *string         `json:"error_message,omitempty"`
	Metadata           json.RawMessage `json:"metadata"`
	Tags               []Tag           `json:"tags"`
	ScheduledAt        *time.Time      `json:"scheduled_at,omitempty"`
	SubmittedAt        *time.Time      `json:"submitted_at,omitempty"`
	DeliveredAt        *time.Time      `json:"delivered_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	DestinationCountry string          `json:"destination_country"`
}

type Destination struct {
	Country string `json:"country"`
}

type SMSResponse struct {
	Object      string          `json:"object"`
	ID          string          `json:"id"`
	MessageID   *string         `json:"message_id"`
	To          string          `json:"to"`
	From        string          `json:"from"`
	Body        string          `json:"body"`
	Status      string          `json:"last_event"`
	Destination Destination     `json:"destination"`
	Segments    int32           `json:"segments"`
	Metadata    json.RawMessage `json:"metadata"`
	Tags        []Tag           `json:"tags"`
	ScheduledAt *time.Time      `json:"scheduled_at"`
	Failure     *SMSFailure     `json:"failure,omitempty"`
	SubmittedAt *time.Time      `json:"submitted_at,omitempty"`
	DeliveredAt *time.Time      `json:"delivered_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type SMSFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (m Message) Response() SMSResponse {
	return SMSResponse{
		Object:      "sms",
		ID:          m.ID,
		MessageID:   m.ProviderMessageID,
		To:          m.To,
		From:        m.From,
		Body:        m.Body,
		Status:      m.Status,
		Destination: Destination{Country: m.DestinationCountry},
		Segments:    m.Segments,
		Metadata:    m.Metadata,
		Tags:        nonNilSMSTags(m.Tags),
		ScheduledAt: m.ScheduledAt,
		Failure:     publicFailure(m.Status),
		SubmittedAt: m.SubmittedAt,
		DeliveredAt: m.DeliveredAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func Responses(messages []Message) []SMSResponse {
	responses := make([]SMSResponse, len(messages))
	for index, message := range messages {
		responses[index] = message.Response()
	}
	return responses
}

func publicFailure(status string) *SMSFailure {
	switch status {
	case StatusUndelivered:
		return &SMSFailure{Code: "SMS_UNDELIVERED", Message: "SMS could not be delivered"}
	case StatusRejected:
		return &SMSFailure{Code: "SMS_REJECTED", Message: "SMS was rejected"}
	case StatusFailed:
		return &SMSFailure{Code: "SMS_FAILED", Message: "SMS delivery failed"}
	case StatusExpired:
		return &SMSFailure{Code: "SMS_EXPIRED", Message: "SMS delivery expired"}
	default:
		return nil
	}
}

type SendRequest struct {
	To                 string          `json:"to"`
	From               string          `json:"from"`
	Body               string          `json:"body"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	Tags               []Tag           `json:"tags,omitempty"`
	ScheduledAt        string          `json:"scheduled_at,omitempty"`
	DestinationCountry string          `json:"-"`
}

type BatchSendRequest struct {
	Messages []SendRequest `json:"messages"`
}

func (request *BatchSendRequest) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		return json.Unmarshal(data, &request.Messages)
	}
	type alias BatchSendRequest
	return json.Unmarshal(data, (*alias)(request))
}

type Tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SendResponse struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}

type UpdateRequest struct {
	ScheduledAt string `json:"scheduled_at"`
}

func (m Message) SendResponse() SendResponse { return SendResponse{Object: "sms", ID: m.ID} }

func SendResponses(messages []Message) []SendResponse {
	responses := make([]SendResponse, len(messages))
	for index, message := range messages {
		responses[index] = message.SendResponse()
	}
	return responses
}

func nonNilSMSTags(tags []Tag) []Tag {
	if tags == nil {
		return []Tag{}
	}
	return tags
}

type ListRequest struct {
	Limit  int32
	Offset int32
}
