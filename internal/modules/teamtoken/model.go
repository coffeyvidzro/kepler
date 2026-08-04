package teamtoken

import "time"

const TokenPrefix = "dgb_team_"

type Token struct {
	ID          string     `json:"id"`
	TeamID      string     `json:"team_id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	Permissions []string   `json:"permissions"`
	CreatedBy   *string    `json:"created_by,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CreatedToken struct {
	Token
	Secret string `json:"secret"`
}

type CreateRequest struct {
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type UpdateRequest struct {
	Name        string     `json:"name"`
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}
