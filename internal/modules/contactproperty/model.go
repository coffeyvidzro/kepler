package contactproperty

import "time"

type Property struct {
	ID            string    `json:"id"`
	TeamID        string    `json:"team_id"`
	Key           string    `json:"key"`
	Type          string    `json:"type"`
	FallbackValue any       `json:"fallback_value,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Key           string `json:"key"`
	Type          string `json:"type"`
	FallbackValue any    `json:"fallback_value,omitempty"`
}

type UpdateRequest struct {
	FallbackValue any `json:"fallback_value"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}
