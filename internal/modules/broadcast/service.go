package broadcast

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	messagetemplate "github.com/coffeyvidzro/dugble/server/internal/modules/messagetemplate"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type TemplateService interface {
	Get(context.Context, string) (messagetemplate.Template, error)
	Preview(context.Context, string, messagetemplate.PreviewRequest) (messagetemplate.PreviewResponse, error)
}

type Service struct {
	repository *Repository
	templates  TemplateService
}

func NewService(repository *Repository, templates TemplateService) *Service {
	return &Service{repository: repository, templates: templates}
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	tc, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed { return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason) }
	return tc, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsWrite)
	if err != nil { return Broadcast{}, err }
	if strings.TrimSpace(req.Name) == "" { return Broadcast{}, apperrors.NewBadRequest("Broadcast name is required") }
	segmentID, err := parseID(req.SegmentID, "Segment id")
	if err != nil { return Broadcast{}, err }
	topicID, err := parseOptionalID(req.TopicID, "Topic id")
	if err != nil { return Broadcast{}, err }
	if s.templates == nil { return Broadcast{}, apperrors.NewInternal("Template service is not configured", nil) }
	template, err := s.templates.Get(ctx, strings.TrimSpace(req.Template))
	if err != nil { return Broadcast{}, err }
	req.Name = strings.TrimSpace(req.Name)
	if req.VariableBindings == nil { req.VariableBindings = map[string]any{} }
	value, err := s.repository.Create(ctx, tc.Scope.TeamID, segmentID, topicID, uuid.MustParse(template.ID), req)
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to create broadcast", err) }
	return value, nil
}

func (s *Service) List(ctx context.Context, req ListRequest) ([]Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsRead)
	if err != nil { return nil, err }
	if req.Limit <= 0 || req.Limit > 100 { req.Limit = 50 }
	if req.Offset < 0 { req.Offset = 0 }
	values, err := s.repository.List(ctx, tc.Scope.TeamID, req.Limit, req.Offset)
	if err != nil { return nil, apperrors.NewInternal("Unable to list broadcasts", err) }
	return values, nil
}

func (s *Service) Get(ctx context.Context, identifier string) (Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsRead)
	if err != nil { return Broadcast{}, err }
	id, err := parseID(identifier, "Broadcast id")
	if err != nil { return Broadcast{}, err }
	value, err := s.repository.Get(ctx, tc.Scope.TeamID, id)
	if errors.Is(err, ErrNotFound) { return Broadcast{}, apperrors.NewNotFound("Broadcast not found") }
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to get broadcast", err) }
	return value, nil
}

func (s *Service) Update(ctx context.Context, identifier string, req UpdateRequest) (Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsWrite)
	if err != nil { return Broadcast{}, err }
	id, err := parseID(identifier, "Broadcast id")
	if err != nil { return Broadcast{}, err }
	if req.Revision <= 0 { return Broadcast{}, apperrors.NewBadRequest("Revision is required") }
	current, err := s.repository.Get(ctx, tc.Scope.TeamID, id)
	if errors.Is(err, ErrNotFound) { return Broadcast{}, apperrors.NewNotFound("Broadcast not found") }
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to get broadcast", err) }
	if current.Status != StatusDraft { return Broadcast{}, apperrors.NewConflict("Only draft broadcasts can be updated") }
	if req.Name != nil {
		current.Name = strings.TrimSpace(*req.Name)
		if current.Name == "" { return Broadcast{}, apperrors.NewBadRequest("Broadcast name is required") }
	}
	segmentID := uuid.MustParse(current.SegmentID)
	if req.SegmentID != nil { segmentID, err = parseID(*req.SegmentID, "Segment id"); if err != nil { return Broadcast{}, err } }
	topicID, err := parseOptionalID(current.TopicID, "Topic id")
	if err != nil { return Broadcast{}, err }
	if req.TopicID != nil { topicID, err = parseOptionalID(*req.TopicID, "Topic id"); if err != nil { return Broadcast{}, err } }
	templateID := uuid.MustParse(current.TemplateID)
	if req.Template != nil {
		template, getErr := s.templates.Get(ctx, strings.TrimSpace(*req.Template))
		if getErr != nil { return Broadcast{}, getErr }
		templateID = uuid.MustParse(template.ID)
		current.TemplateID = template.ID
	}
	if req.VariableBindings != nil { current.VariableBindings = *req.VariableBindings }
	value, err := s.repository.Update(ctx, tc.Scope.TeamID, id, segmentID, topicID, templateID, req, current)
	if errors.Is(err, ErrConflict) { return Broadcast{}, apperrors.NewConflict("Broadcast was modified or is no longer a draft") }
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to update broadcast", err) }
	return value, nil
}

func (s *Service) Delete(ctx context.Context, identifier string) (Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsWrite)
	if err != nil { return Broadcast{}, err }
	id, err := parseID(identifier, "Broadcast id")
	if err != nil { return Broadcast{}, err }
	value, err := s.repository.Delete(ctx, tc.Scope.TeamID, id)
	if errors.Is(err, ErrConflict) { return Broadcast{}, apperrors.NewConflict("Only draft or canceled broadcasts can be deleted") }
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to delete broadcast", err) }
	return value, nil
}

func (s *Service) Send(ctx context.Context, identifier string, req SendRequest) (Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsSend)
	if err != nil { return Broadcast{}, err }
	id, err := parseID(identifier, "Broadcast id")
	if err != nil { return Broadcast{}, err }
	current, err := s.repository.Get(ctx, tc.Scope.TeamID, id)
	if errors.Is(err, ErrNotFound) { return Broadcast{}, apperrors.NewNotFound("Broadcast not found") }
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to get broadcast", err) }
	template, err := s.templates.Get(ctx, current.TemplateID)
	if err != nil { return Broadcast{}, err }
	if template.PublishedVersionID == nil { return Broadcast{}, apperrors.NewConflict("Broadcast template must have a published version") }
	if req.ScheduledAt != nil && !req.ScheduledAt.After(time.Now()) { return Broadcast{}, apperrors.NewBadRequest("scheduled_at must be in the future") }
	value, err := s.repository.Send(ctx, tc.Scope.TeamID, id, uuid.MustParse(*template.PublishedVersionID), req.ScheduledAt)
	if errors.Is(err, ErrConflict) { return Broadcast{}, apperrors.NewConflict("Only draft broadcasts can be sent") }
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to send broadcast", err) }
	return value, nil
}

func (s *Service) Cancel(ctx context.Context, identifier string) (Broadcast, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsSend)
	if err != nil { return Broadcast{}, err }
	id, err := parseID(identifier, "Broadcast id")
	if err != nil { return Broadcast{}, err }
	value, err := s.repository.Cancel(ctx, tc.Scope.TeamID, id)
	if errors.Is(err, ErrConflict) { return Broadcast{}, apperrors.NewConflict("Only scheduled broadcasts can be canceled") }
	if err != nil { return Broadcast{}, apperrors.NewInternal("Unable to cancel broadcast", err) }
	return value, nil
}

func (s *Service) Preview(ctx context.Context, identifier string, req PreviewRequest) (messagetemplate.PreviewResponse, error) {
	value, err := s.Get(ctx, identifier)
	if err != nil { return messagetemplate.PreviewResponse{}, err }
	return s.templates.Preview(ctx, value.TemplateID, messagetemplate.PreviewRequest{VersionID: pointerValue(value.TemplateVersionID), Variables: req.Variables})
}

func (s *Service) ListRecipients(ctx context.Context, identifier string, req ListRequest) ([]Recipient, error) {
	tc, err := requireTenant(ctx, tenant.PermissionBroadcastsRead)
	if err != nil { return nil, err }
	id, err := parseID(identifier, "Broadcast id")
	if err != nil { return nil, err }
	if req.Limit <= 0 || req.Limit > 100 { req.Limit = 50 }
	if req.Offset < 0 { req.Offset = 0 }
	if _, err = s.repository.Get(ctx, tc.Scope.TeamID, id); errors.Is(err, ErrNotFound) { return nil, apperrors.NewNotFound("Broadcast not found") }
	values, err := s.repository.ListRecipients(ctx, tc.Scope.TeamID, id, req.Limit, req.Offset)
	if err != nil { return nil, apperrors.NewInternal("Unable to list broadcast recipients", err) }
	return values, nil
}

func parseID(value, label string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil { return uuid.Nil, apperrors.NewBadRequest(label+" must be a valid UUID") }
	return id, nil
}
func parseOptionalID(value *string, label string) (*uuid.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" { return nil, nil }
	id, err := parseID(*value, label)
	if err != nil { return nil, err }
	return &id, nil
}
func pointerValue(value *string) string { if value == nil { return "" }; return *value }
