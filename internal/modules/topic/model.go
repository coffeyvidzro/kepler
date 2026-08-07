package topic

import "time"

type Topic struct {
	ID                  string    `json:"id"`
	TeamID              string    `json:"team_id"`
	Name                string    `json:"name"`
	Description         *string   `json:"description,omitempty"`
	DefaultSubscription string    `json:"default_subscription"`
	Visibility          string    `json:"visibility"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Name                string  `json:"name"`
	Description         *string `json:"description,omitempty"`
	DefaultSubscription string  `json:"default_subscription"`
	Visibility          string  `json:"visibility,omitempty"`
}

type UpdateRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description **string `json:"description,omitempty"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}
