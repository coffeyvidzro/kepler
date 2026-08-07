package suppression

import "time"

type Suppression struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	Email     string    `json:"email"`
	Origin    string    `json:"origin"`
	SourceID  *string   `json:"source_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateRequest struct {
	Email string `json:"email"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}
