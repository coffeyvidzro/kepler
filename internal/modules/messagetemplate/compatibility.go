package messagetemplate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	ObjectTemplate = "template"
	ObjectList     = "list"
	maxAPIPerPage  = 100
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

func (s *Service) CreateAPI(ctx context.Context, request APICreateRequest) (MutationResponse, error) {
	mapped, err := mapAPICreateRequest(request)
	if err != nil {
		return MutationResponse{}, err
	}
	value, err := s.Create(ctx, mapped)
	if err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Object: ObjectTemplate, ID: value.ID}, nil
}

func (s *Service) ListAPI(ctx context.Context, request APIListRequest) (ListResponse, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesRead)
	if err != nil {
		return ListResponse{}, err
	}
	if err := normalizeAPIListRequest(&request); err != nil {
		return ListResponse{}, err
	}
	after, err := parseTemplateCursor(request.After)
	if err != nil {
		return ListResponse{}, err
	}
	before, err := parseTemplateCursor(request.Before)
	if err != nil {
		return ListResponse{}, err
	}
	cursor := after
	if cursor == nil {
		cursor = before
	}
	if cursor != nil {
		exists, lookupErr := s.repository.CursorExists(ctx, access.Scope.TeamID, *cursor)
		if lookupErr != nil {
			return ListResponse{}, apperrors.NewInternal("Unable to validate template cursor", lookupErr)
		}
		if !exists {
			return ListResponse{}, apperrors.NewNotFound("Template cursor not found")
		}
	}
	values, err := s.repository.ListPage(ctx, access.Scope.TeamID, request.Limit+1, after, before)
	if err != nil {
		return ListResponse{}, apperrors.NewInternal("Unable to list templates", err)
	}
	hasMore := len(values) > int(request.Limit)
	if hasMore {
		values = values[:request.Limit]
	}
	if before != nil {
		slices.Reverse(values)
	}
	data := make([]ListItem, 0, len(values))
	for _, value := range values {
		data = append(data, ListItem{
			ID: value.ID, Name: value.Name, Status: templateStatus(value),
			PublishedAt: value.PublishedAt, CreatedAt: value.CreatedAt,
			UpdatedAt: value.UpdatedAt, Alias: value.Alias,
		})
	}
	return ListResponse{Object: ObjectList, Data: data, HasMore: hasMore}, nil
}

func (s *Service) GetAPI(ctx context.Context, identifier string) (Resource, error) {
	template, err := s.Get(ctx, identifier)
	if err != nil {
		return Resource{}, err
	}
	if template.CurrentVersionID == nil {
		return Resource{}, apperrors.NewConflict("Template has no current version")
	}
	version, err := s.GetVersion(ctx, identifier, *template.CurrentVersionID)
	if err != nil {
		return Resource{}, err
	}
	return resourceFromTemplate(template, version)
}

func (s *Service) UpdateAPI(ctx context.Context, identifier string, request APIUpdateRequest) (MutationResponse, error) {
	current, err := s.Get(ctx, identifier)
	if err != nil {
		return MutationResponse{}, err
	}
	if current.CurrentVersionID == nil {
		return MutationResponse{}, apperrors.NewConflict("Template has no current version")
	}
	mapped := UpdateRequest{BaseVersionID: *current.CurrentVersionID}
	mapped.Name = request.Name
	mapped.Alias = request.Alias
	mapped.Subject = request.Subject
	mapped.HTML = request.HTML
	mapped.Variables = request.Variables
	if request.Text != nil {
		value := request.Text
		mapped.Text = &value
	}
	if request.From != nil {
		email, name, parseErr := splitSender(request.From)
		if parseErr != nil {
			return MutationResponse{}, parseErr
		}
		mapped.FromEmail = &email
		mapped.FromName = &name
	}
	if request.ReplyTo != nil {
		values, normalizeErr := normalizeReplyTo([]string(*request.ReplyTo))
		if normalizeErr != nil {
			return MutationResponse{}, normalizeErr
		}
		encoded, encodeErr := encodeStoredReplyTo(values)
		if encodeErr != nil {
			return MutationResponse{}, apperrors.NewInternal("Unable to encode reply-to addresses", encodeErr)
		}
		mapped.ReplyTo = &encoded
	}
	value, err := s.Update(ctx, identifier, mapped)
	if err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Object: ObjectTemplate, ID: value.ID}, nil
}

func (s *Service) DeleteAPI(ctx context.Context, identifier string) (DeleteResponse, error) {
	value, err := s.Delete(ctx, identifier)
	if err != nil {
		return DeleteResponse{}, err
	}
	return DeleteResponse{Object: ObjectTemplate, ID: value.ID, Deleted: true}, nil
}

func (s *Service) PublishAPI(ctx context.Context, identifier string) (MutationResponse, error) {
	value, err := s.Publish(ctx, identifier, PublishRequest{})
	if err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Object: ObjectTemplate, ID: value.ID}, nil
}

func (s *Service) DuplicateAPI(ctx context.Context, identifier string) (MutationResponse, error) {
	value, err := s.Duplicate(ctx, identifier, DuplicateRequest{})
	if err != nil {
		return MutationResponse{}, err
	}
	return MutationResponse{Object: ObjectTemplate, ID: value.ID}, nil
}

func mapAPICreateRequest(request APICreateRequest) (CreateRequest, error) {
	email, name, err := splitSender(request.From)
	if err != nil {
		return CreateRequest{}, err
	}
	replyTo, err := normalizeReplyTo([]string(request.ReplyTo))
	if err != nil {
		return CreateRequest{}, err
	}
	storedReplyTo, err := encodeStoredReplyTo(replyTo)
	if err != nil {
		return CreateRequest{}, apperrors.NewInternal("Unable to encode reply-to addresses", err)
	}
	subject := ""
	if request.Subject != nil {
		subject = *request.Subject
	}
	return CreateRequest{
		Name: request.Name, Alias: request.Alias, FromEmail: email, FromName: name,
		ReplyTo: storedReplyTo, Subject: subject, HTML: request.HTML,
		Text: request.Text, Variables: request.Variables,
	}, nil
}

func resourceFromTemplate(template Template, version Version) (Resource, error) {
	variables := make([]VariableResource, 0, len(version.Variables))
	versionID, err := uuid.Parse(version.ID)
	if err != nil {
		return Resource{}, apperrors.NewInternal("Unable to parse template version", err)
	}
	for _, variable := range version.Variables {
		variables = append(variables, VariableResource{
			ID: uuid.NewSHA1(versionID, []byte(variable.Key)).String(),
			Key: variable.Key, Type: variable.Type, FallbackValue: variable.FallbackValue,
			CreatedAt: version.CreatedAt, UpdatedAt: version.CreatedAt,
		})
	}
	replyTo, err := decodeStoredReplyTo(version.ReplyToEmail)
	if err != nil {
		return Resource{}, apperrors.NewInternal("Unable to decode reply-to addresses", err)
	}
	return Resource{
		Object: ObjectTemplate, ID: template.ID, CurrentVersionID: version.ID,
		Alias: template.Alias, Name: template.Name, CreatedAt: template.CreatedAt,
		UpdatedAt: template.UpdatedAt, Status: templateStatus(template),
		PublishedAt: template.PublishedAt, From: formatSender(version),
		Subject: optionalNonEmpty(version.Subject), ReplyTo: replyTo,
		HTML: version.HTML, Text: version.Text, Variables: variables,
		HasUnpublishedVersions: template.HasUnpublishedChanges,
	}, nil
}

func templateStatus(template Template) string {
	if template.PublishedVersionID != nil {
		return "published"
	}
	return "draft"
}

func normalizeAPIListRequest(request *APIListRequest) error {
	if request.Limit == 0 {
		request.Limit = 20
	}
	if request.Limit < 1 || request.Limit > maxAPIPerPage {
		return apperrors.NewBadRequest("Limit must be between 1 and 100")
	}
	request.After = strings.TrimSpace(request.After)
	request.Before = strings.TrimSpace(request.Before)
	if request.After != "" && request.Before != "" {
		return apperrors.NewBadRequest("After and before cannot be used together")
	}
	return nil
}

func parseTemplateCursor(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, apperrors.NewBadRequest("Template cursor must be a valid UUID")
	}
	return &id, nil
}

func splitSender(value *string) (*string, *string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil, nil
	}
	address, err := mail.ParseAddress(strings.TrimSpace(*value))
	if err != nil || address.Address == "" {
		return nil, nil, apperrors.NewBadRequest("From must be a valid email address")
	}
	email := strings.ToLower(address.Address)
	var name *string
	if strings.TrimSpace(address.Name) != "" {
		text := strings.TrimSpace(address.Name)
		name = &text
	}
	return &email, name, nil
}

func formatSender(version Version) *string {
	if version.FromEmail == nil {
		return nil
	}
	value := *version.FromEmail
	if version.FromName != nil && strings.TrimSpace(*version.FromName) != "" {
		value = (&mail.Address{Name: *version.FromName, Address: *version.FromEmail}).String()
	}
	return &value
}

func normalizeReplyTo(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		address, err := mail.ParseAddress(strings.TrimSpace(value))
		if err != nil || address.Address == "" {
			return nil, apperrors.NewBadRequest("Reply-to must contain valid email addresses")
		}
		address.Address = strings.ToLower(address.Address)
		result = append(result, address.String())
	}
	return result, nil
}

func encodeStoredReplyTo(values []string) (*string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) == 1 {
		value := values[0]
		return &value, nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	value := string(encoded)
	return &value, nil
}

func decodeStoredReplyTo(value *string) ([]string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if !strings.HasPrefix(trimmed, "[") {
		return []string{trimmed}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func optionalNonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
