package messagetemplate

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type EmailSender interface {
	Send(context.Context, emailmodule.SendRequest) (emailmodule.Message, error)
}

type Service struct {
	repository *Repository
	email      EmailSender
}

func NewService(repository *Repository, dependencies ...any) *Service {
	service := &Service{repository: repository}
	for _, dependency := range dependencies {
		if sender, ok := dependency.(EmailSender); ok {
			service.email = sender
		}
	}
	return service
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
	if request.Alias != nil {
		mapped.Alias = &request.Alias
	}
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
			ID:  uuid.NewSHA1(versionID, []byte(variable.Key)).String(),
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

func optionalNonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesWrite)
	if err != nil {
		return Template{}, err
	}
	req, err = validateCreate(req)
	if err != nil {
		return Template{}, err
	}
	template, _, err := s.repository.Create(ctx, access.Scope.TeamID, req)
	if errors.Is(err, ErrAliasConflict) {
		return Template{}, apperrors.NewConflict("A template with this alias already exists")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to create template", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "template.created", ResourceType: "message_template", ResourceID: template.ID})
	return template, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesRead)
	if err != nil {
		return nil, err
	}
	normalizeList(&req)
	values, err := s.repository.List(ctx, access.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list templates", err)
	}
	return values, nil
}

func (s *Service) Get(ctx context.Context, identifier string) (Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesRead)
	if err != nil {
		return Template{}, err
	}
	return s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
}

func (s *Service) Update(ctx context.Context, identifier string, req UpdateRequest) (Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesWrite)
	if err != nil {
		return Template{}, err
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return Template{}, err
	}
	baseID, err := uuid.Parse(strings.TrimSpace(req.BaseVersionID))
	if err != nil {
		return Template{}, apperrors.NewBadRequest("base_version_id must be a valid UUID")
	}
	base, err := s.repository.GetVersion(ctx, access.Scope.TeamID, uuid.MustParse(template.ID), baseID)
	if errors.Is(err, ErrVersionNotFound) {
		return Template{}, apperrors.NewConflict("The template draft has changed; reload before updating")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to load template version", err)
	}
	if err = validateUpdate(template, base, &req); err != nil {
		return Template{}, err
	}
	updated, _, err := s.repository.Update(ctx, access.Scope.TeamID, template, base, req)
	switch {
	case errors.Is(err, ErrVersionConflict):
		return Template{}, apperrors.NewConflict("The template draft has changed; reload before updating")
	case errors.Is(err, ErrAliasConflict):
		return Template{}, apperrors.NewConflict("A template with this alias already exists")
	case err != nil:
		return Template{}, apperrors.NewInternal("Unable to update template", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "template.updated", ResourceType: "message_template", ResourceID: updated.ID})
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, identifier string) (Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesWrite)
	if err != nil {
		return Template{}, err
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return Template{}, err
	}
	deleted, err := s.repository.Delete(ctx, access.Scope.TeamID, uuid.MustParse(template.ID))
	if errors.Is(err, ErrNotFound) {
		return Template{}, apperrors.NewNotFound("Template not found")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to delete template", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "template.deleted", ResourceType: "message_template", ResourceID: deleted.ID})
	return deleted, nil
}

func (s *Service) Publish(ctx context.Context, identifier string, req PublishRequest) (Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesWrite)
	if err != nil {
		return Template{}, err
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return Template{}, err
	}
	versionID := template.CurrentVersionID
	if strings.TrimSpace(req.VersionID) != "" {
		value, parseErr := uuid.Parse(req.VersionID)
		if parseErr != nil {
			return Template{}, apperrors.NewBadRequest("version_id must be a valid UUID")
		}
		text := value.String()
		versionID = &text
	}
	if versionID == nil {
		return Template{}, apperrors.NewConflict("Template has no version to publish")
	}
	version, err := s.repository.GetVersion(ctx, access.Scope.TeamID, uuid.MustParse(template.ID), uuid.MustParse(*versionID))
	if errors.Is(err, ErrVersionNotFound) {
		return Template{}, apperrors.NewNotFound("Template version not found")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to load template version", err)
	}
	if err = validateVersion(version); err != nil {
		return Template{}, err
	}
	published, err := s.repository.Publish(ctx, access.Scope.TeamID, uuid.MustParse(template.ID), uuid.MustParse(version.ID))
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to publish template", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "template.published", ResourceType: "message_template", ResourceID: published.ID, Metadata: map[string]any{"version_id": version.ID}})
	return published, nil
}

func (s *Service) Duplicate(ctx context.Context, identifier string, req DuplicateRequest) (Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesWrite)
	if err != nil {
		return Template{}, err
	}
	source, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return Template{}, err
	}
	if source.CurrentVersionID == nil {
		return Template{}, apperrors.NewConflict("Template has no version to duplicate")
	}
	version, err := s.repository.GetVersion(ctx, access.Scope.TeamID, uuid.MustParse(source.ID), uuid.MustParse(*source.CurrentVersionID))
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to load template version", err)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = source.Name + " Copy"
	}
	create := CreateRequest{Name: name, Alias: req.Alias, FromEmail: version.FromEmail, FromName: version.FromName, ReplyTo: version.ReplyToEmail, Subject: version.Subject, HTML: version.HTML, Text: version.Text, Variables: version.Variables}
	create, err = validateCreate(create)
	if err != nil {
		return Template{}, err
	}
	copy, _, err := s.repository.Create(ctx, access.Scope.TeamID, create)
	if errors.Is(err, ErrAliasConflict) {
		return Template{}, apperrors.NewConflict("A template with this alias already exists")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to duplicate template", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "template.duplicated", ResourceType: "message_template", ResourceID: copy.ID, Metadata: map[string]any{"source_template_id": source.ID}})
	return copy, nil
}

func (s *Service) ListVersions(ctx context.Context, identifier string, req ListRequest) ([]Version, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesRead)
	if err != nil {
		return nil, err
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return nil, err
	}
	normalizeList(&req)
	values, err := s.repository.ListVersions(ctx, access.Scope.TeamID, uuid.MustParse(template.ID), req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list template versions", err)
	}
	return values, nil
}

func (s *Service) GetVersion(ctx context.Context, identifier, versionValue string) (Version, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesRead)
	if err != nil {
		return Version{}, err
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return Version{}, err
	}
	versionID, err := uuid.Parse(strings.TrimSpace(versionValue))
	if err != nil {
		return Version{}, apperrors.NewBadRequest("version_id must be a valid UUID")
	}
	version, err := s.repository.GetVersion(ctx, access.Scope.TeamID, uuid.MustParse(template.ID), versionID)
	if errors.Is(err, ErrVersionNotFound) {
		return Version{}, apperrors.NewNotFound("Template version not found")
	}
	if err != nil {
		return Version{}, apperrors.NewInternal("Unable to get template version", err)
	}
	return version, nil
}

func (s *Service) Revert(ctx context.Context, identifier, versionValue string) (Template, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesWrite)
	if err != nil {
		return Template{}, err
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return Template{}, err
	}
	if template.CurrentVersionID == nil {
		return Template{}, apperrors.NewConflict("Template has no current version")
	}
	targetID, err := uuid.Parse(strings.TrimSpace(versionValue))
	if err != nil {
		return Template{}, apperrors.NewBadRequest("version_id must be a valid UUID")
	}
	target, err := s.repository.GetVersion(ctx, access.Scope.TeamID, uuid.MustParse(template.ID), targetID)
	if errors.Is(err, ErrVersionNotFound) {
		return Template{}, apperrors.NewNotFound("Template version not found")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to load template version", err)
	}
	base, err := s.repository.GetVersion(ctx, access.Scope.TeamID, uuid.MustParse(template.ID), uuid.MustParse(*template.CurrentVersionID))
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to load current template version", err)
	}
	fromEmail, fromName, replyTo, textBody := target.FromEmail, target.FromName, target.ReplyToEmail, target.Text
	subject, htmlBody, variables := target.Subject, target.HTML, target.Variables
	note := "Reverted from version " + fmt.Sprint(target.VersionNumber)
	request := UpdateRequest{BaseVersionID: base.ID, FromEmail: &fromEmail, FromName: &fromName, ReplyTo: &replyTo, Subject: &subject, HTML: &htmlBody, Text: &textBody, Variables: &variables, ChangeNote: &note}
	updated, _, err := s.repository.Update(ctx, access.Scope.TeamID, template, base, request)
	if errors.Is(err, ErrVersionConflict) {
		return Template{}, apperrors.NewConflict("The template draft has changed; reload before reverting")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to revert template", err)
	}
	audit.Record(ctx, access, audit.Event{Action: "template.reverted", ResourceType: "message_template", ResourceID: updated.ID, Metadata: map[string]any{"source_version_id": target.ID}})
	return updated, nil
}

func (s *Service) Preview(ctx context.Context, identifier string, req PreviewRequest) (PreviewResponse, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesRead)
	if err != nil {
		return PreviewResponse{}, err
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return PreviewResponse{}, err
	}
	version, err := s.resolveVersion(ctx, access.Scope.TeamID, template, req.VersionID)
	if err != nil {
		return PreviewResponse{}, err
	}
	result, err := Render(version, req.Variables)
	if err != nil {
		return PreviewResponse{}, apperrors.NewBadRequest(err.Error())
	}
	return result, nil
}

func (s *Service) RenderVersionTx(ctx context.Context, tx pgx.Tx, teamID, templateID, versionID uuid.UUID, variables map[string]any) (PreviewResponse, error) {
	if s == nil || s.repository == nil {
		return PreviewResponse{}, errors.New("message template repository is not configured")
	}
	if tx == nil {
		return PreviewResponse{}, errors.New("message template transaction is not configured")
	}
	version, err := s.repository.GetVersionTx(ctx, tx, teamID, templateID, versionID)
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("load pinned message template version: %w", err)
	}
	rendered, err := Render(version, variables)
	if err != nil {
		return PreviewResponse{}, fmt.Errorf("render pinned message template version: %w", err)
	}
	return rendered, nil
}

func (s *Service) TestSend(ctx context.Context, identifier string, req TestSendRequest) (emailmodule.SendResponse, error) {
	access, err := requireAccess(ctx, tenant.PermissionTemplatesWrite)
	if err != nil {
		return emailmodule.SendResponse{}, err
	}
	if s.email == nil {
		return emailmodule.SendResponse{}, apperrors.NewInternal("Template test email sender is not configured", nil)
	}
	template, err := s.resolveTemplate(ctx, access.Scope.TeamID, identifier)
	if err != nil {
		return emailmodule.SendResponse{}, err
	}
	version, err := s.resolveVersion(ctx, access.Scope.TeamID, template, req.VersionID)
	if err != nil {
		return emailmodule.SendResponse{}, err
	}
	preview, err := Render(version, req.Variables)
	if err != nil {
		return emailmodule.SendResponse{}, apperrors.NewBadRequest(err.Error())
	}
	request := emailmodule.SendRequest{To: emailmodule.EmailAddressList{{Email: req.To}}, Subject: preview.Subject, HTML: preview.HTML, Stream: emailmodule.MessageTypeTransactional}
	if preview.Text != nil {
		request.Text = *preview.Text
	}
	if preview.FromEmail != nil {
		request.From = &emailmodule.EmailAddress{Email: *preview.FromEmail}
		if preview.FromName != nil {
			request.From.Name = *preview.FromName
		}
	}
	if preview.ReplyTo != nil {
		request.ReplyTo = emailmodule.EmailAddressList{{Email: *preview.ReplyTo}}
	}
	message, err := s.email.Send(ctx, request)
	if err != nil {
		return emailmodule.SendResponse{}, err
	}
	audit.Record(ctx, access, audit.Event{Action: "template.test_sent", ResourceType: "message_template", ResourceID: template.ID, Metadata: map[string]any{"version_id": version.ID, "email_id": message.ID}})
	return emailmodule.SendResponse{ID: message.ID}, nil
}

func (s *Service) resolveTemplate(ctx context.Context, teamID uuid.UUID, identifier string) (Template, error) {
	value, err := s.repository.Resolve(ctx, teamID, strings.TrimSpace(identifier))
	if errors.Is(err, ErrNotFound) {
		return Template{}, apperrors.NewNotFound("Template not found")
	}
	if err != nil {
		return Template{}, apperrors.NewInternal("Unable to resolve template", err)
	}
	return value, nil
}
func (s *Service) resolveVersion(ctx context.Context, teamID uuid.UUID, template Template, requested string) (Version, error) {
	versionID := template.CurrentVersionID
	if strings.TrimSpace(requested) != "" {
		id, err := uuid.Parse(requested)
		if err != nil {
			return Version{}, apperrors.NewBadRequest("version_id must be a valid UUID")
		}
		value := id.String()
		versionID = &value
	}
	if versionID == nil {
		return Version{}, apperrors.NewConflict("Template has no version")
	}
	version, err := s.repository.GetVersion(ctx, teamID, uuid.MustParse(template.ID), uuid.MustParse(*versionID))
	if errors.Is(err, ErrVersionNotFound) {
		return Version{}, apperrors.NewNotFound("Template version not found")
	}
	if err != nil {
		return Version{}, apperrors.NewInternal("Unable to load template version", err)
	}
	return version, nil
}

func requireAccess(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return access, nil
}
