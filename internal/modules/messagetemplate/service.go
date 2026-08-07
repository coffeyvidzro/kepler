package messagetemplate

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const (
	maxTemplateNameCharacters  = 100
	maxTemplateAliasCharacters = 100
	maxTemplateVariables       = 50
	maxTemplateSubjectChars    = 255
)

var (
	aliasPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	variableKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,49}$`)
	reservedVariables  = map[string]struct{}{
		"FIRST_NAME": {}, "LAST_NAME": {}, "EMAIL": {}, "UNSUBSCRIBE_URL": {}, "RESEND_UNSUBSCRIBE_URL": {},
		"CONTACT": {}, "THIS": {},
	}
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

func validateCreate(req CreateRequest) (CreateRequest, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Alias = normalizeOptional(req.Alias)
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > maxTemplateNameCharacters {
		return req, apperrors.NewBadRequest("Template name is required and must be at most 100 characters")
	}
	if err := validateAlias(req.Alias); err != nil {
		return req, err
	}
	if err := validateContent(req.Subject, req.HTML, req.FromEmail, req.ReplyTo, req.Variables); err != nil {
		return req, err
	}
	req.FromEmail = normalizeOptional(req.FromEmail)
	req.FromName = normalizeOptional(req.FromName)
	return req, nil
}

func validateUpdate(template Template, base Version, req *UpdateRequest) error {
	name, alias := template.Name, template.Alias
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		req.Name = &name
	}
	if req.Alias != nil {
		alias = normalizeOptional(*req.Alias)
		req.Alias = &alias
	}
	if name == "" || utf8.RuneCountInString(name) > maxTemplateNameCharacters {
		return apperrors.NewBadRequest("Template name is required and must be at most 100 characters")
	}
	if err := validateAlias(alias); err != nil {
		return err
	}
	subject, htmlBody, variables := base.Subject, base.HTML, base.Variables
	fromEmail, replyTo := base.FromEmail, base.ReplyToEmail
	if req.Subject != nil {
		subject = strings.TrimSpace(*req.Subject)
		req.Subject = &subject
	}
	if req.HTML != nil {
		htmlBody = *req.HTML
	}
	if req.Variables != nil {
		variables = *req.Variables
	}
	if req.FromEmail != nil {
		fromEmail = *req.FromEmail
	}
	if req.ReplyTo != nil {
		replyTo = *req.ReplyTo
	}
	return validateContent(subject, htmlBody, fromEmail, replyTo, variables)
}

func validateVersion(version Version) error {
	if strings.TrimSpace(version.Subject) == "" {
		return apperrors.NewBadRequest("Template subject is required before publishing")
	}
	return validateContent(version.Subject, version.HTML, version.FromEmail, version.ReplyToEmail, version.Variables)
}

func validateAlias(alias *string) error {
	if alias == nil {
		return nil
	}
	if len(*alias) > maxTemplateAliasCharacters || !aliasPattern.MatchString(*alias) {
		return apperrors.NewBadRequest("Template alias must use letters, numbers, underscores, or dashes and be at most 100 characters")
	}
	return nil
}

func validateContent(subject, htmlBody string, fromEmail, replyTo *string, variables []Variable) error {
	if utf8.RuneCountInString(subject) > maxTemplateSubjectChars {
		return apperrors.NewBadRequest("Template subject must be at most 255 characters")
	}
	if strings.TrimSpace(htmlBody) == "" {
		return apperrors.NewBadRequest("Template HTML is required")
	}
	if len(variables) > maxTemplateVariables {
		return apperrors.NewBadRequest("Template may define at most 50 variables")
	}
	definitions := map[string]struct{}{}
	for _, variable := range variables {
		if !variableKeyPattern.MatchString(variable.Key) {
			return apperrors.NewBadRequest("Template variable keys must start with a letter and use only letters, numbers, and underscores")
		}
		upper := strings.ToUpper(variable.Key)
		if _, reserved := reservedVariables[upper]; reserved {
			return apperrors.NewBadRequest("Template variable key is reserved: " + variable.Key)
		}
		if _, exists := definitions[variable.Key]; exists {
			return apperrors.NewBadRequest("Template variable keys must be unique")
		}
		definitions[variable.Key] = struct{}{}
		if variable.Type != VariableTypeString && variable.Type != VariableTypeNumber {
			return apperrors.NewBadRequest("Template variable type must be string or number")
		}
		if variable.FallbackValue != nil {
			if _, err := renderVariableValue(variable, variable.FallbackValue); err != nil {
				return apperrors.NewBadRequest(err.Error())
			}
		}
	}
	for _, key := range referencedVariables(subject, htmlBody) {
		if _, exists := definitions[key]; !exists {
			return apperrors.NewBadRequest("Unknown template variable: " + key)
		}
	}
	if err := validateStoredEmail(fromEmail, "Template sender"); err != nil {
		return err
	}
	if replyTo != nil {
		values, err := decodeStoredReplyTo(replyTo)
		if err != nil {
			return apperrors.NewBadRequest("Template reply-to addresses are invalid")
		}
		for _, value := range values {
			address, parseErr := mail.ParseAddress(strings.TrimSpace(value))
			if parseErr != nil || address.Address == "" {
				return apperrors.NewBadRequest("Template reply-to addresses are invalid")
			}
		}
	}
	return nil
}

func validateStoredEmail(value *string, label string) error {
	if value == nil {
		return nil
	}
	parsed, err := mail.ParseAddress(strings.TrimSpace(*value))
	if err != nil || !strings.EqualFold(parsed.Address, strings.TrimSpace(*value)) {
		return apperrors.NewBadRequest(label + " must be a valid email address")
	}
	return nil
}

func normalizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeList(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
}

func requireAccess(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	access, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return access, nil
}
