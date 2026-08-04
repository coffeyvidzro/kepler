package auditevent

import "time"

type Event struct {
	ID             string         `json:"id"`
	TeamID         string         `json:"team_id"`
	ActorType      string         `json:"actor_type"`
	ActorUserID    *string        `json:"actor_user_id,omitempty"`
	ActorSessionID *string        `json:"actor_session_id,omitempty"`
	ActorTokenID   *string        `json:"actor_token_id,omitempty"`
	Action         string         `json:"action"`
	ResourceType   string         `json:"resource_type"`
	ResourceID     string         `json:"resource_id"`
	Outcome        string         `json:"outcome"`
	Metadata       map[string]any `json:"metadata"`
	RequestID      *string        `json:"request_id,omitempty"`
	IPAddress      *string        `json:"ip_address,omitempty"`
	UserAgent      *string        `json:"user_agent,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}

type ListResponse struct {
	Events     []Event `json:"events"`
	NextCursor *string `json:"next_cursor,omitempty"`
}
