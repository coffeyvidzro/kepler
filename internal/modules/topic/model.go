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
	Visibility  *string  `json:"visibility,omitempty"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}

const (
	ObjectTopic = "topic"
	ObjectList  = "list"
)

type APIListRequest struct {
	Limit  int32
	After  string
	Before string
}

type MutationResponse struct {
	Object string `json:"object"`
	ID     string `json:"id"`
}

type DeleteResponse struct {
	Object  string `json:"object"`
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

type Resource struct {
	Object              string    `json:"object"`
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         *string   `json:"description"`
	DefaultSubscription string    `json:"default_subscription"`
	Visibility          string    `json:"visibility"`
	CreatedAt           time.Time `json:"created_at"`
}

type ListResponse struct {
	Object  string     `json:"object"`
	HasMore bool       `json:"has_more"`
	Data    []Resource `json:"data"`
}
