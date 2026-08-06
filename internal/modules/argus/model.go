package argus

import (
	"encoding/json"
	"time"
)

const (
	ChannelEmail = "email"
	ChannelSMS   = "sms"
)

const (
	StatusPending            = "pending"
	StatusApproved           = "approved"
	StatusExpired            = "expired"
	StatusCanceled           = "canceled"
	StatusMaxAttemptsReached = "max_attempts_reached"
	StatusDeliveryFailed     = "delivery_failed"
)

type Verification struct {
	ID                    string          `json:"id"`
	TeamID                string          `json:"team_id"`
	Channel               string          `json:"channel"`
	Recipient             string          `json:"recipient"`
	CodeLength            int32           `json:"code_length"`
	TTLSeconds            int32           `json:"ttl_seconds"`
	MaxAttempts           int32           `json:"max_attempts"`
	ResendCooldownSeconds int32           `json:"-"`
	MaxResends            int32           `json:"max_resends"`
	Status                string          `json:"status"`
	Locale                *string         `json:"locale,omitempty"`
	Metadata              json.RawMessage `json:"metadata"`
	AttemptCount          int32           `json:"attempt_count"`
	ResendCount           int32           `json:"resend_count"`
	ExpiresAt             time.Time       `json:"expires_at"`
	ApprovedAt            *time.Time      `json:"approved_at,omitempty"`
	ExpiredAt             *time.Time      `json:"expired_at,omitempty"`
	CanceledAt            *time.Time      `json:"canceled_at,omitempty"`
	FailedAt              *time.Time      `json:"failed_at,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type CreateVerificationRequest struct {
	Recipient   string          `json:"recipient"`
	Channel     string          `json:"channel,omitempty"`
	CodeLength  *int32          `json:"code_length,omitempty"`
	TTLSeconds  *int32          `json:"ttl_seconds,omitempty"`
	MaxAttempts *int32          `json:"max_attempts,omitempty"`
	MaxResends  *int32          `json:"max_resends,omitempty"`
	Locale      *string         `json:"locale,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type CheckRequest struct {
	Code      string          `json:"code"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	UserAgent *string         `json:"-"`
	IPHash    []byte          `json:"-"`
}

type CheckResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Valid   bool   `json:"valid"`
	Expired bool   `json:"expired"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}
