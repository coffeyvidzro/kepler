package broadcast

import "time"

const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusQueued    = "queued"
	StatusSent      = "sent"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

type Broadcast struct {
	ID                string         `json:"id"`
	TeamID            string         `json:"team_id"`
	Name              string         `json:"name"`
	Status            string         `json:"status"`
	SegmentID         string         `json:"segment_id"`
	TopicID           *string        `json:"topic_id,omitempty"`
	TemplateID        string         `json:"template_id"`
	TemplateVersionID *string        `json:"template_version_id,omitempty"`
	VariableBindings  map[string]any `json:"variable_bindings"`
	ScheduledAt       *time.Time     `json:"scheduled_at,omitempty"`
	QueuedAt          *time.Time     `json:"queued_at,omitempty"`
	SentAt            *time.Time     `json:"sent_at,omitempty"`
	CanceledAt        *time.Time     `json:"canceled_at,omitempty"`
	AudienceCount     int64          `json:"audience_count"`
	EligibleCount     int64          `json:"eligible_count"`
	SuppressedCount   int64          `json:"suppressed_count"`
	QueuedCount       int64          `json:"queued_count"`
	FailedCount       int64          `json:"failed_count"`
	Revision          int64          `json:"revision"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type CreateRequest struct {
	Name             string         `json:"name"`
	SegmentID        string         `json:"segment_id"`
	TopicID          *string        `json:"topic_id,omitempty"`
	Template         string         `json:"template"`
	VariableBindings map[string]any `json:"variable_bindings,omitempty"`
}

type UpdateRequest struct {
	Revision         int64           `json:"revision"`
	Name             *string         `json:"name,omitempty"`
	SegmentID        *string         `json:"segment_id,omitempty"`
	TopicID          **string        `json:"topic_id,omitempty"`
	Template         *string         `json:"template,omitempty"`
	VariableBindings *map[string]any `json:"variable_bindings,omitempty"`
}

type SendRequest struct {
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

type PreviewRequest struct {
	Variables map[string]any `json:"variables,omitempty"`
}

type ListRequest struct {
	Limit  int32
	Offset int32
}

type Recipient struct {
	ID              string         `json:"id"`
	BroadcastID     string         `json:"broadcast_id"`
	ContactID       *string        `json:"contact_id,omitempty"`
	Email           string         `json:"email"`
	FirstName       *string        `json:"first_name,omitempty"`
	LastName        *string        `json:"last_name,omitempty"`
	ContactSnapshot map[string]any `json:"contact_snapshot"`
	Status          string         `json:"status"`
	ExclusionReason *string        `json:"exclusion_reason,omitempty"`
	EmailMessageID  *string        `json:"email_message_id,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	QueuedAt        *time.Time     `json:"queued_at,omitempty"`
}
