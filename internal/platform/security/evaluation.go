package security

import (
	"net/netip"

	"github.com/google/uuid"
)

// Evaluation contains the context required to evaluate a security action.
type Evaluation struct {
	Action   Action
	Request  Request
	Identity Identity
	Metadata map[string]string
}

// Request contains transport-neutral request attributes.
type Request struct {
	Method    string
	Path      string
	Host      string
	UserAgent string
	IPAddress netip.Addr
	RequestID string
}

// Identity contains authenticated and anonymous security principals.
type Identity struct {
	UserID    *uuid.UUID
	TeamID    *uuid.UUID
	APIKeyID  *uuid.UUID
	SessionID *uuid.UUID
}
