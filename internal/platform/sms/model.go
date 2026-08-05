package sms

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

const MaxSenderIDCharacters = 11

const (
	StatusQueued      = "queued"
	StatusSubmitted   = "submitted"
	StatusSent        = "sent"
	StatusDelivered   = "delivered"
	StatusUndelivered = "undelivered"
	StatusRejected    = "rejected"
	StatusFailed      = "failed"
	StatusExpired     = "expired"
	StatusUnknown     = "unknown"
)

type SendRequest struct {
	Reference          string
	To                 string
	From               string
	Message            string
	DestinationCountry string
}

func (request SendRequest) Normalize() SendRequest {
	request.Reference = strings.TrimSpace(request.Reference)
	request.To = strings.TrimSpace(request.To)
	request.From = strings.TrimSpace(request.From)
	request.DestinationCountry = NormalizeCountryCode(request.DestinationCountry)
	if request.DestinationCountry == "" {
		country, err := ResolveDestinationCountry(request.To)
		if err == nil {
			request.DestinationCountry = country
		}
	}
	return request
}

func (request SendRequest) Validate() error {
	request = request.Normalize()
	if request.To == "" {
		return &ValidationError{Field: "to", Reason: "recipient is required"}
	}
	if request.From == "" {
		return &ValidationError{Field: "from", Reason: "sender ID is required"}
	}
	if utf8.RuneCountInString(request.From) > MaxSenderIDCharacters {
		return &ValidationError{Field: "from", Reason: fmt.Sprintf("sender ID must not exceed %d characters", MaxSenderIDCharacters)}
	}
	if strings.TrimSpace(request.Message) == "" {
		return &ValidationError{Field: "message", Reason: "message is required"}
	}
	resolvedCountry, err := ResolveDestinationCountry(request.To)
	if err != nil {
		return &ValidationError{Field: "to", Reason: "destination country is not supported"}
	}
	if !IsCountryCode(request.DestinationCountry) {
		return &ValidationError{Field: "destination_country", Reason: "destination country is required"}
	}
	if request.DestinationCountry != resolvedCountry {
		return &ValidationError{Field: "destination_country", Reason: "destination country does not match recipient"}
	}
	return nil
}

type SendResponse struct {
	ProviderID    string
	ProviderMsgID string
	Status        string
}

type StatusResponse struct {
	ProviderID     string
	ProviderMsgID  string
	Status         string
	ProviderStatus string
}

type Provider interface {
	ID() string
	Send(context.Context, SendRequest) (*SendResponse, error)
	CheckStatus(context.Context, string) (*StatusResponse, error)
}

type Router interface {
	Route(context.Context, string) ([]string, error)
	ShouldFallback(context.Context, string, error) bool
}

type ValidationError struct {
	Field  string
	Reason string
}

func (err *ValidationError) Error() string {
	if err == nil {
		return "invalid SMS request"
	}
	if err.Field == "" {
		return "invalid SMS request: " + err.Reason
	}
	return fmt.Sprintf("invalid SMS request field %q: %s", err.Field, err.Reason)
}

type ProviderAttempt struct {
	ProviderID string
	Err        error
}

type SendError struct {
	Attempts []ProviderAttempt
}

func (err *SendError) Error() string {
	if err == nil || len(err.Attempts) == 0 {
		return "SMS send failed"
	}
	last := err.Attempts[len(err.Attempts)-1]
	if last.Err == nil {
		return fmt.Sprintf("SMS send failed via %s", last.ProviderID)
	}
	return fmt.Sprintf("SMS send failed via %s: %v", last.ProviderID, last.Err)
}

func (err *SendError) Unwrap() []error {
	if err == nil {
		return nil
	}
	causes := make([]error, 0, len(err.Attempts))
	for _, attempt := range err.Attempts {
		if attempt.Err != nil {
			causes = append(causes, attempt.Err)
		}
	}
	return causes
}

func IsKnownStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusQueued, StatusSubmitted, StatusSent, StatusDelivered, StatusUndelivered,
		StatusRejected, StatusFailed, StatusExpired, StatusUnknown:
		return true
	default:
		return false
	}
}
