package messagetemplate

import (
	"encoding/json"
	"time"
)

const (
	VariableTypeString = "string"
	VariableTypeNumber = "number"
)

type Template struct {
	ID                    string     `json:"id"`
	TeamID                string     `json:"team_id"`
	Name                  string     `json:"name"`
	Alias                 string     `json:"alias"`
	CurrentVersionID      *string    `json:"current_version_id,omitempty"`
	PublishedVersionID    *string    `json:"published_version_id,omitempty"`
	PublishedAt           *time.Time `json:"published_at,omitempty"`
	HasUnpublishedChanges bool       `json:"has_unpublished_changes"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type Version struct {
	ID               string     `json:"id"`
	TeamID           string     `json:"team_id"`
	TemplateID       string     `json:"template_id"`
	VersionNumber    int32      `json:"version_number"`
	FromEmail        *string    `json:"from_email,omitempty"`
	FromName         *string    `json:"from_name,omitempty"`
	ReplyToEmail     *string    `json:"reply_to_email,omitempty"`
	Subject          string     `json:"subject"`
	HTML             string     `json:"html"`
	Text             *string    `json:"text,omitempty"`
	Variables        []Variable `json:"variables"`
	BasedOnVersionID *string    `json:"based_on_version_id,omitempty"`
	ChangeNote       *string    `json:"change_note,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type Variable struct {
	Key           string `json:"key"`
	Type          string `json:"type"`
	FallbackValue any    `json:"fallback_value,omitempty"`
}

type CreateRequest struct {
	Name      string     `json:"name"`
	Alias     string     `json:"alias"`
	FromEmail *string    `json:"from_email,omitempty"`
	FromName  *string    `json:"from_name,omitempty"`
	ReplyTo   *string    `json:"reply_to,omitempty"`
	Subject   string     `json:"subject"`
	HTML      string     `json:"html"`
	Text      *string    `json:"text,omitempty"`
	Variables []Variable `json:"variables,omitempty"`
}

type UpdateRequest struct {
	BaseVersionID string      `json:"base_version_id"`
	Name          *string     `json:"name,omitempty"`
	Alias         *string     `json:"alias,omitempty"`
	FromEmail     **string    `json:"from_email,omitempty"`
	FromName      **string    `json:"from_name,omitempty"`
	ReplyTo       **string    `json:"reply_to,omitempty"`
	Subject       *string     `json:"subject,omitempty"`
	HTML          *string     `json:"html,omitempty"`
	Text          **string    `json:"text,omitempty"`
	Variables     *[]Variable `json:"variables,omitempty"`
	ChangeNote    *string     `json:"change_note,omitempty"`
}

type DuplicateRequest struct {
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

type PublishRequest struct {
	VersionID string `json:"version_id,omitempty"`
}

type PreviewRequest struct {
	VersionID string         `json:"version_id,omitempty"`
	Variables map[string]any `json:"variables,omitempty"`
}

type PreviewResponse struct {
	TemplateID string  `json:"template_id"`
	VersionID  string  `json:"version_id"`
	Subject    string  `json:"subject"`
	HTML       string  `json:"html"`
	Text       *string `json:"text,omitempty"`
	FromEmail  *string `json:"from_email,omitempty"`
	FromName   *string `json:"from_name,omitempty"`
	ReplyTo    *string `json:"reply_to,omitempty"`
}

type TestSendRequest struct {
	To        string         `json:"to"`
	VersionID string         `json:"version_id,omitempty"`
	Variables map[string]any `json:"variables,omitempty"`
}

type ListRequest struct{ Limit, Offset int32 }

func encodeVariables(value []Variable) ([]byte, error) {
	if value == nil {
		value = []Variable{}
	}
	return json.Marshal(value)
}
