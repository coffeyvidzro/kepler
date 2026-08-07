package segment

import "time"

type Segment struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Contact struct {
	ID           string    `json:"id"`
	TeamID       string    `json:"team_id"`
	Email        string    `json:"email"`
	FirstName    *string   `json:"first_name,omitempty"`
	LastName     *string   `json:"last_name,omitempty"`
	Unsubscribed bool      `json:"unsubscribed"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Name string `json:"name"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}
