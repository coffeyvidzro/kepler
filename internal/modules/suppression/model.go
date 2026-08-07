package suppression

import "time"

const (
	ObjectSuppression = "suppression"
	ObjectList        = "list"
	maxBatchSize      = 100
)

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

type BatchAddRequest struct {
	Emails []string `json:"emails"`
}

type BatchRemoveRequest struct {
	Emails []string `json:"emails,omitempty"`
	IDs    []string `json:"ids,omitempty"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}

type APIListRequest struct {
	Limit  int32
	After  string
	Before string
	Origin string
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
	Object    string    `json:"object"`
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Origin    string    `json:"origin"`
	SourceID  *string   `json:"source_id"`
	CreatedAt time.Time `json:"created_at"`
}

type ListResponse struct {
	Object  string     `json:"object"`
	HasMore bool       `json:"has_more"`
	Data    []Resource `json:"data"`
}

type BatchAddResponse struct {
	Data []MutationResponse `json:"data"`
}

type BatchRemoveResponse struct {
	Data []DeleteResponse `json:"data"`
}
