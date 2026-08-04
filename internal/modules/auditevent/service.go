package auditevent

import (
	"context"

	"github.com/google/uuid"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const defaultPageSize int32 = 50
const maxPageSize int32 = 100

type Service struct{ repository *audit.Repository }

func NewService(repository *audit.Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, before string, requested int32) (ListResponse, error) {
	access, decision := tenant.ResolveAccess(ctx, tenant.PermissionAuditEventsRead)
	if !decision.Allowed {
		return ListResponse{}, apperrors.NewForbidden(decision.Reason)
	}
	pageSize := requested
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		return ListResponse{}, apperrors.NewBadRequest("Audit event page size cannot exceed 100")
	}
	var beforeID *uuid.UUID
	if before != "" {
		parsed, err := uuid.Parse(before)
		if err != nil {
			return ListResponse{}, apperrors.NewBadRequest("Audit event cursor must be a valid UUID")
		}
		beforeID = &parsed
	}
	entries, err := s.repository.ListTeam(ctx, access.Scope.TeamID, beforeID, pageSize)
	if err != nil {
		return ListResponse{}, apperrors.NewInternal("Unable to list audit events", err)
	}
	events := make([]Event, 0, len(entries))
	for _, entry := range entries {
		events = append(events, eventFromEntry(entry))
	}
	response := ListResponse{Events: events}
	if len(events) == int(pageSize) {
		cursor := events[len(events)-1].ID
		response.NextCursor = &cursor
	}
	return response, nil
}

func eventFromEntry(entry audit.Entry) Event {
	return Event{ID: entry.ID.String(), TeamID: entry.TeamID.String(), ActorType: entry.ActorType, ActorUserID: uuidString(entry.ActorUserID), ActorSessionID: stringValue(entry.ActorSessionID), ActorTokenID: uuidString(entry.ActorTokenID), Action: entry.Action, ResourceType: entry.ResourceType, ResourceID: entry.ResourceID, Outcome: entry.Outcome, Metadata: entry.Metadata, RequestID: stringValue(entry.Request.RequestID), IPAddress: stringValue(entry.Request.IPAddress), UserAgent: stringValue(entry.Request.UserAgent), CreatedAt: entry.CreatedAt}
}

func uuidString(value uuid.UUID) *string {
	if value == uuid.Nil {
		return nil
	}
	result := value.String()
	return &result
}
func stringValue(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
