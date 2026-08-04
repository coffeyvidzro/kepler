package team

import "time"

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

const (
	InvitationStatusPending  = "pending"
	InvitationStatusAccepted = "accepted"
	InvitationStatusDeclined = "declined"
	InvitationStatusRevoked  = "revoked"
)

type Team struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	MarketCode string    `json:"market_code"`
	Phone      string    `json:"phone"`
	Address    string    `json:"address"`
	Website    *string   `json:"website,omitempty"`
	Status     string    `json:"status"`
	CreatedBy  *string   `json:"created_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Member struct {
	TeamID    string    `json:"team_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Name       string `json:"name"`
	MarketCode string `json:"market_code"`
	Phone      string `json:"phone"`
	Address    string `json:"address"`
	Website    string `json:"website"`
}

type UpdateRequest struct {
	Name string `json:"name"`
}

type UpdateMemberRoleRequest struct {
	Role string `json:"role"`
}

type Invitation struct {
	ID         string     `json:"id"`
	TeamID     string     `json:"team_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	Status     string     `json:"status"`
	InvitedBy  *string    `json:"invited_by,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	DeclinedAt *time.Time `json:"declined_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Token      string     `json:"token,omitempty"`
}

type InviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}
