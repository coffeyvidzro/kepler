package tenant

import (
	"context"

	"github.com/google/uuid"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"

	StatusActive    = "active"
	StatusDisabled  = "disabled"
	StatusSuspended = "suspended"
	StatusInvited   = "invited"
)

type Membership struct {
	TeamID     uuid.UUID
	UserID     uuid.UUID
	Role       string
	Status     string
	TeamStatus string
}

func (m Membership) Active() bool {
	return m.Status == StatusActive && (m.TeamStatus == "" || m.TeamStatus == StatusActive)
}

type MembershipStore interface {
	GetTenantMembership(ctx context.Context, teamID uuid.UUID, userID uuid.UUID) (Membership, error)
}
