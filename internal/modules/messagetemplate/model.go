package messagetemplate

import (
	"encoding/json"
	"fmt"
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
	Alias                 *string    `json:"alias"`
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
	Alias     *string    `json:"alias,omitempty"`
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
	Alias         **string    `json:"alias,omitempty"`
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
	Name  string  `json:"name,omitempty"`
	Alias *string `json:"alias,omitempty"`
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

const (
	ObjectTemplate = "template"
	ObjectList     = "list"
)

type StringList []string

func (values *StringList) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*values = nil
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*values = StringList{single}
		return nil
	}
	var multiple []string
	if err := json.Unmarshal(data, &multiple); err != nil {
		return fmt.Errorf("must be a string or an array of strings")
	}
	*values = multiple
	return nil
}

type APICreateRequest struct {
	Name      string     `json:"name"`
	HTML      string     `json:"html"`
	Alias     *string    `json:"alias,omitempty"`
	From      *string    `json:"from,omitempty"`
	Subject   *string    `json:"subject,omitempty"`
	ReplyTo   StringList `json:"reply_to,omitempty"`
	Text      *string    `json:"text,omitempty"`
	Variables []Variable `json:"variables,omitempty"`
}

type APIUpdateRequest struct {
	Name      *string     `json:"name,omitempty"`
	HTML      *string     `json:"html,omitempty"`
	Alias     *string     `json:"alias,omitempty"`
	From      *string     `json:"from,omitempty"`
	Subject   *string     `json:"subject,omitempty"`
	ReplyTo   *StringList `json:"reply_to,omitempty"`
	Text      *string     `json:"text,omitempty"`
	Variables *[]Variable `json:"variables,omitempty"`
}

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

type VariableResource struct {
	ID            string    `json:"id"`
	Key           string    `json:"key"`
	Type          string    `json:"type"`
	FallbackValue any       `json:"fallback_value"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Resource struct {
	Object                 string             `json:"object"`
	ID                     string             `json:"id"`
	CurrentVersionID       string             `json:"current_version_id"`
	Alias                  *string            `json:"alias"`
	Name                   string             `json:"name"`
	CreatedAt              time.Time          `json:"created_at"`
	UpdatedAt              time.Time          `json:"updated_at"`
	Status                 string             `json:"status"`
	PublishedAt            *time.Time         `json:"published_at"`
	From                   *string            `json:"from"`
	Subject                *string            `json:"subject"`
	ReplyTo                []string           `json:"reply_to"`
	HTML                   string             `json:"html"`
	Text                   *string            `json:"text"`
	Variables              []VariableResource `json:"variables"`
	HasUnpublishedVersions bool               `json:"has_unpublished_versions"`
}

type ListItem struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Alias       *string    `json:"alias"`
}

type ListResponse struct {
	Object  string     `json:"object"`
	Data    []ListItem `json:"data"`
	HasMore bool       `json:"has_more"`
}
