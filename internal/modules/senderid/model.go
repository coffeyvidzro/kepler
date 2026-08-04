package senderid

import "time"

const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusSuspended = "suspended"
	StatusInactive  = "inactive"
)

type SenderID struct {
	ID              string     `json:"id"`
	TeamID          string     `json:"team_id"`
	Name            string     `json:"name"`
	CountryCode     string     `json:"country_code"`
	Purpose         string     `json:"purpose"`
	Status          string     `json:"status"`
	Provider        *string    `json:"provider,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	ApprovedAt      *time.Time `json:"approved_at,omitempty"`
	RejectedAt      *time.Time `json:"rejected_at,omitempty"`
	SuspendedAt     *time.Time `json:"suspended_at,omitempty"`
	CreatedBy       *string    `json:"created_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type CreateRequest struct {
	Name        string  `json:"name"`
	CountryCode string  `json:"country_code"`
	Purpose     string  `json:"purpose"`
	Provider    *string `json:"provider,omitempty"`
}
