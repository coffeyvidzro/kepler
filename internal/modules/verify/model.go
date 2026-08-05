package verify

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

type VerificationService struct {
	ID                    string          `json:"id"`
	TeamID                string          `json:"team_id"`
	Key                   string          `json:"key"`
	Name                  string          `json:"name"`
	DefaultChannel        string          `json:"default_channel"`
	CodeLength            int32           `json:"code_length"`
	TTLSeconds            int32           `json:"ttl_seconds"`
	MaxAttempts           int32           `json:"max_attempts"`
	ResendCooldownSeconds int32           `json:"resend_cooldown_seconds"`
	MaxResends            int32           `json:"max_resends"`
	Enabled               bool            `json:"enabled"`
	Metadata              json.RawMessage `json:"metadata"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type CreateServiceRequest struct {
	Key                   string          `json:"key"`
	Name                  string          `json:"name"`
	DefaultChannel        string          `json:"default_channel,omitempty"`
	CodeLength            int32           `json:"code_length,omitempty"`
	TTLSeconds            int32           `json:"ttl_seconds,omitempty"`
	MaxAttempts           int32           `json:"max_attempts,omitempty"`
	ResendCooldownSeconds *int32          `json:"resend_cooldown_seconds,omitempty"`
	MaxResends            *int32          `json:"max_resends,omitempty"`
	Enabled               *bool           `json:"enabled,omitempty"`
	Metadata              json.RawMessage `json:"metadata,omitempty"`
}

type UpdateServiceRequest struct {
	Name                  *string          `json:"name,omitempty"`
	DefaultChannel        *string          `json:"default_channel,omitempty"`
	CodeLength            *int32           `json:"code_length,omitempty"`
	TTLSeconds            *int32           `json:"ttl_seconds,omitempty"`
	MaxAttempts           *int32           `json:"max_attempts,omitempty"`
	ResendCooldownSeconds *int32           `json:"resend_cooldown_seconds,omitempty"`
	MaxResends            *int32           `json:"max_resends,omitempty"`
	Enabled               *bool            `json:"enabled,omitempty"`
	Metadata              *json.RawMessage `json:"metadata,omitempty"`
}

type Verification struct {
	ID           string          `json:"id"`
	TeamID       string          `json:"team_id"`
	ServiceID    string          `json:"service_id"`
	Channel      string          `json:"channel"`
	Recipient    string          `json:"recipient"`
	Status       string          `json:"status"`
	Locale       *string         `json:"locale,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	AttemptCount int32           `json:"attempt_count"`
	ResendCount  int32           `json:"resend_count"`
	ExpiresAt    time.Time       `json:"expires_at"`
	ApprovedAt   *time.Time      `json:"approved_at,omitempty"`
	ExpiredAt    *time.Time      `json:"expired_at,omitempty"`
	CanceledAt   *time.Time      `json:"canceled_at,omitempty"`
	FailedAt     *time.Time      `json:"failed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type CreateVerificationRequest struct {
	ServiceID string          `json:"service_id,omitempty"`
	Service   string          `json:"service,omitempty"`
	Channel   string          `json:"channel,omitempty"`
	Recipient string          `json:"recipient"`
	Locale    *string         `json:"locale,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
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
