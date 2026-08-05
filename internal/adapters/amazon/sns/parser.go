package sns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type MessageType string

const (
	TypeNotification             MessageType = "Notification"
	TypeSubscriptionConfirmation MessageType = "SubscriptionConfirmation"
	TypeUnsubscribeConfirmation  MessageType = "UnsubscribeConfirmation"
)

type Envelope struct {
	Type             MessageType `json:"Type"`
	MessageID        string      `json:"MessageId"`
	TopicARN         string      `json:"TopicArn"`
	Subject          *string     `json:"Subject,omitempty"`
	Message          string      `json:"Message"`
	Timestamp        string      `json:"Timestamp"`
	SignatureVersion string      `json:"SignatureVersion"`
	Signature        string      `json:"Signature"`
	SigningCertURL   string      `json:"SigningCertURL"`
	SubscribeURL     *string     `json:"SubscribeURL,omitempty"`
	Token            *string     `json:"Token,omitempty"`
	UnsubscribeURL   *string     `json:"UnsubscribeURL,omitempty"`
}

func ParseEnvelope(body []byte) (Envelope, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return Envelope{}, fmt.Errorf("%w: request body is empty", ErrInvalidEnvelope)
	}

	var envelope Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Envelope{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidEnvelope, err)
	}
	if err := validateEnvelope(envelope); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

func validateEnvelope(envelope Envelope) error {
	required := []struct {
		name  string
		value string
	}{
		{name: "Type", value: string(envelope.Type)},
		{name: "MessageId", value: envelope.MessageID},
		{name: "TopicArn", value: envelope.TopicARN},
		{name: "Message", value: envelope.Message},
		{name: "Timestamp", value: envelope.Timestamp},
		{name: "SignatureVersion", value: envelope.SignatureVersion},
		{name: "Signature", value: envelope.Signature},
		{name: "SigningCertURL", value: envelope.SigningCertURL},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidEnvelope, field.name)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, envelope.Timestamp); err != nil {
		return fmt.Errorf("%w: Timestamp must be RFC3339: %v", ErrInvalidEnvelope, err)
	}

	switch envelope.Type {
	case TypeNotification:
		return nil
	case TypeSubscriptionConfirmation, TypeUnsubscribeConfirmation:
		if envelope.SubscribeURL == nil || strings.TrimSpace(*envelope.SubscribeURL) == "" {
			return fmt.Errorf("%w: SubscribeURL is required for %s", ErrInvalidEnvelope, envelope.Type)
		}
		if envelope.Token == nil || strings.TrimSpace(*envelope.Token) == "" {
			return fmt.Errorf("%w: Token is required for %s", ErrInvalidEnvelope, envelope.Type)
		}
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedMessageType, envelope.Type)
	}
}
