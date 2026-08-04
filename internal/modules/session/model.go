package session

import (
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
)

type Authentication struct {
	CredentialVersion int64
	Method            authnz.AuthenticationMethod
	Assurance         authnz.AssuranceLevel
	AuthenticatedAt   time.Time
	MFACompletedAt    *time.Time
}

type Record struct {
	ID         string
	UserID     uuid.UUID
	TokenHash  string
	UserAgent  *string
	IPAddress  *string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	LastSeenAt time.Time
	Authentication
}

type Session struct {
	ID                   string                      `json:"id"`
	UserAgent            *string                     `json:"user_agent,omitempty"`
	IPAddress            *string                     `json:"ip_address,omitempty"`
	ExpiresAt            time.Time                   `json:"expires_at"`
	RevokedAt            *time.Time                  `json:"revoked_at,omitempty"`
	CreatedAt            time.Time                   `json:"created_at"`
	LastSeenAt           time.Time                   `json:"last_seen_at"`
	AuthenticationMethod authnz.AuthenticationMethod `json:"authentication_method"`
	AssuranceLevel       authnz.AssuranceLevel       `json:"assurance_level"`
	AuthenticatedAt      time.Time                   `json:"authenticated_at"`
	MFACompletedAt       *time.Time                  `json:"mfa_completed_at,omitempty"`
}
