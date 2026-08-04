package authnz

import (
	"time"

	"github.com/google/uuid"
)

type Principal struct {
	UserID               uuid.UUID
	SessionID            string
	Email                string
	Name                 string
	EmailVerified        bool
	CredentialVersion    int64
	AuthenticationMethod AuthenticationMethod
	AssuranceLevel       AssuranceLevel
	AuthenticatedAt      time.Time
	MFACompletedAt       *time.Time
}
