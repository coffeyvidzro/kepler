package sms

import (
	"context"
	"errors"
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

var (
	ErrRouterRequired       = errors.New("sms router is required")
	ErrNoProviderAvailable  = errors.New("no SMS provider is available")
	ErrProviderNotFound     = errors.New("SMS provider not found")
	ErrInvalidProviderReply = errors.New("invalid SMS provider response")
)

// SendRequest is Dugble's provider-neutral request for one recipient.
// DestinationCountry is resolved by Dugble from To and is never client chosen.
type SendRequest struct {
	To                 string
	From               string
	Message            string
	DestinationCountry string
}

// Normalize trims routing fields while preserving the message exactly as the
// caller supplied it. When the country snapshot is omitted it is derived from
// the recipient number.
func (r SendRequest) Normalize() SendRequest {
	r.To = strings.TrimSpace(r.To)
	r.From = strings.TrimSpace(r.From)
	r.DestinationCountry = NormalizeCountryCode(r.DestinationCountry)
	if r.DestinationCountry == "" {
		country, err := ResolveDestinationCountry(r.To)
		if err == nil {
			r.DestinationCountry = country
		}
	}
	return r
}

func (r SendRequest) Validate() error {
	r = r.Normalize()

	if r.To == "" {
		return &ValidationError{Field: "to", Reason: "recipient is required"}
	}
	if r.From == "" {
		return &ValidationError{Field: "from", Reason: "sender ID is required"}
	}
	if utf8.RuneCountInString(r.From) > MaxSenderIDCharacters {
		return &ValidationError{
			Field:  "from",
			Reason: fmt.Sprintf("sender ID must not exceed %d characters", MaxSenderIDCharacters),
		}
	}
	if strings.TrimSpace(r.Message) == "" {
		return &ValidationError{Field: "message", Reason: "message is required"}
	}

	resolvedCountry, err := ResolveDestinationCountry(r.To)
	if err != nil {
		return &ValidationError{Field: "to", Reason: "destination country is not supported"}
	}
	if !IsCountryCode(r.DestinationCountry) {
		return &ValidationError{Field: "destination_country", Reason: "destination country is required"}
	}
	if r.DestinationCountry != resolvedCountry {
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
	ProviderID    string
	ProviderMsgID string
	Status        string
}

// Provider is implemented by every upstream SMS adapter.
type Provider interface {
	ID() string
	Send(ctx context.Context, req SendRequest) (*SendResponse, error)
	CheckStatus(ctx context.Context, providerMessageID string) (*StatusResponse, error)
}

// Router selects exactly one provider for a destination country and supports
// lookup of the original provider for delivery-status checks.
type Router interface {
	Route(ctx context.Context, req SendRequest) (Provider, error)
	Provider(providerID string) (Provider, bool)
}

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "invalid SMS request"
	}
	if e.Field == "" {
		return "invalid SMS request: " + e.Reason
	}
	return fmt.Sprintf("invalid SMS request field %q: %s", e.Field, e.Reason)
}

// ProviderAttempt records a failed upstream submission.
type ProviderAttempt struct {
	ProviderID string
	Err        error
}

// SendError reports the provider submission failure. errors.Is/errors.As can
// inspect the underlying error through Unwrap.
type SendError struct {
	Attempts []ProviderAttempt
}

func (e *SendError) Error() string {
	if e == nil || len(e.Attempts) == 0 {
		return "SMS send failed"
	}

	last := e.Attempts[len(e.Attempts)-1]
	if last.Err == nil {
		return fmt.Sprintf("SMS send failed via %s", last.ProviderID)
	}
	return fmt.Sprintf("SMS send failed via %s: %v", last.ProviderID, last.Err)
}

func (e *SendError) Unwrap() []error {
	if e == nil {
		return nil
	}

	errs := make([]error, 0, len(e.Attempts))
	for _, attempt := range e.Attempts {
		if attempt.Err != nil {
			errs = append(errs, attempt.Err)
		}
	}
	return errs
}

func IsKnownStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusQueued,
		StatusSubmitted,
		StatusSent,
		StatusDelivered,
		StatusUndelivered,
		StatusRejected,
		StatusFailed,
		StatusExpired,
		StatusUnknown:
		return true
	default:
		return false
	}
}
