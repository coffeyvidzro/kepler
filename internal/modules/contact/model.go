package contact

import "time"

type Contact struct {
	ID           string         `json:"id"`
	TeamID       string         `json:"team_id"`
	Email        string         `json:"email"`
	FirstName    *string        `json:"first_name,omitempty"`
	LastName     *string        `json:"last_name,omitempty"`
	Unsubscribed bool           `json:"unsubscribed"`
	Properties   map[string]any `json:"properties"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type CreateRequest struct {
	Email        string         `json:"email"`
	FirstName    *string        `json:"first_name,omitempty"`
	LastName     *string        `json:"last_name,omitempty"`
	Unsubscribed bool           `json:"unsubscribed"`
	Properties   map[string]any `json:"properties,omitempty"`
}

type UpdateRequest struct {
	Email        *string         `json:"email,omitempty"`
	FirstName    **string        `json:"first_name,omitempty"`
	LastName     **string        `json:"last_name,omitempty"`
	Unsubscribed *bool           `json:"unsubscribed,omitempty"`
	Properties   *map[string]any `json:"properties,omitempty"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}
