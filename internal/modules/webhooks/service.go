package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type Service struct {
	repository *Repository
	emitter    *platformwebhook.Emitter
	now        func() time.Time
}

func NewService(repository *Repository, emitter *platformwebhook.Emitter) *Service {
	return &Service{repository: repository, emitter: emitter, now: time.Now}
}

func (s *Service) CreateEndpoint(ctx context.Context, req CreateEndpointRequest) (CreatedEndpoint, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksWrite)
	if err != nil {
		return CreatedEndpoint{}, err
	}
	validated, err := validateCreateEndpoint(req)
	if err != nil {
		return CreatedEndpoint{}, err
	}
	secret, err := platformwebhook.NewSigningSecret()
	if err != nil {
		return CreatedEndpoint{}, apperrors.NewInternal("Unable to generate webhook signing secret", err)
	}
	endpoint, err := s.repository.CreateEndpoint(ctx, tenantContext.Scope.TeamID, validated, []byte(secret))
	if err != nil {
		return CreatedEndpoint{}, apperrors.NewInternal("Unable to create webhook endpoint", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "webhook_endpoint.created", ResourceType: "webhook_endpoint", ResourceID: endpoint.ID})
	return CreatedEndpoint{Endpoint: endpoint, SigningSecret: secret}, nil
}

func (s *Service) ListEndpoints(ctx context.Context, req ListRequest) ([]Endpoint, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	endpoints, err := s.repository.ListEndpoints(ctx, tenantContext.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list webhook endpoints", err)
	}
	return endpoints, nil
}

func (s *Service) GetEndpoint(ctx context.Context, value string) (Endpoint, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksRead)
	if err != nil {
		return Endpoint{}, err
	}
	id, err := parseID(value, "Webhook endpoint")
	if err != nil {
		return Endpoint{}, err
	}
	endpoint, err := s.repository.GetEndpoint(ctx, id, tenantContext.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Endpoint{}, apperrors.NewNotFound("Webhook endpoint not found")
	}
	if err != nil {
		return Endpoint{}, apperrors.NewInternal("Unable to get webhook endpoint", err)
	}
	return endpoint, nil
}

func (s *Service) UpdateEndpoint(ctx context.Context, value string, req UpdateEndpointRequest) (Endpoint, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksWrite)
	if err != nil {
		return Endpoint{}, err
	}
	id, err := parseID(value, "Webhook endpoint")
	if err != nil {
		return Endpoint{}, err
	}
	current, err := s.repository.GetEndpoint(ctx, id, tenantContext.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Endpoint{}, apperrors.NewNotFound("Webhook endpoint not found")
	}
	if err != nil {
		return Endpoint{}, apperrors.NewInternal("Unable to get webhook endpoint", err)
	}
	validated, err := validateUpdateEndpoint(current, req)
	if err != nil {
		return Endpoint{}, err
	}
	endpoint, err := s.repository.UpdateEndpoint(ctx, id, tenantContext.Scope.TeamID, validated)
	if errors.Is(err, pgx.ErrNoRows) {
		return Endpoint{}, apperrors.NewNotFound("Webhook endpoint not found")
	}
	if err != nil {
		return Endpoint{}, apperrors.NewInternal("Unable to update webhook endpoint", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "webhook_endpoint.updated", ResourceType: "webhook_endpoint", ResourceID: endpoint.ID})
	return endpoint, nil
}

func (s *Service) DeleteEndpoint(ctx context.Context, value string) error {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksWrite)
	if err != nil {
		return err
	}
	id, err := parseID(value, "Webhook endpoint")
	if err != nil {
		return err
	}
	endpoint, err := s.repository.DisableEndpoint(ctx, id, tenantContext.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NewNotFound("Webhook endpoint not found")
	}
	if err != nil {
		return apperrors.NewInternal("Unable to disable webhook endpoint", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "webhook_endpoint.disabled", ResourceType: "webhook_endpoint", ResourceID: endpoint.ID})
	return nil
}

func (s *Service) RotateSecret(ctx context.Context, value string) (CreatedEndpoint, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksWrite)
	if err != nil {
		return CreatedEndpoint{}, err
	}
	id, err := parseID(value, "Webhook endpoint")
	if err != nil {
		return CreatedEndpoint{}, err
	}
	secret, err := platformwebhook.NewSigningSecret()
	if err != nil {
		return CreatedEndpoint{}, apperrors.NewInternal("Unable to generate webhook signing secret", err)
	}
	endpoint, err := s.repository.RotateSecret(ctx, id, tenantContext.Scope.TeamID, []byte(secret))
	if errors.Is(err, pgx.ErrNoRows) {
		return CreatedEndpoint{}, apperrors.NewNotFound("Webhook endpoint not found")
	}
	if err != nil {
		return CreatedEndpoint{}, apperrors.NewInternal("Unable to rotate webhook signing secret", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "webhook_endpoint.secret_rotated", ResourceType: "webhook_endpoint", ResourceID: endpoint.ID})
	return CreatedEndpoint{Endpoint: endpoint, SigningSecret: secret}, nil
}

func (s *Service) TestEndpoint(ctx context.Context, value string) (Delivery, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksWrite)
	if err != nil {
		return Delivery{}, err
	}
	endpointID, err := parseID(value, "Webhook endpoint")
	if err != nil {
		return Delivery{}, err
	}
	endpoint, err := s.repository.GetEndpoint(ctx, endpointID, tenantContext.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, apperrors.NewNotFound("Webhook endpoint not found")
	}
	if err != nil {
		return Delivery{}, apperrors.NewInternal("Unable to get webhook endpoint", err)
	}
	if !endpoint.Enabled {
		return Delivery{}, apperrors.NewBadRequest("Webhook endpoint must be enabled before testing")
	}
	payload, err := json.Marshal(struct {
		Test    bool   `json:"test"`
		Message string `json:"message"`
	}{Test: true, Message: "This is a test webhook from Dugble."})
	if err != nil {
		return Delivery{}, apperrors.NewInternal("Unable to encode webhook test payload", err)
	}

	var deliveryID uuid.UUID
	err = s.repository.InTransaction(ctx, func(tx pgx.Tx) error {
		_, createdDeliveryID, emitErr := s.emitter.EmitToEndpointTx(ctx, tx, platformwebhook.Event{
			ID:         uuid.New(),
			TeamID:     tenantContext.Scope.TeamID,
			Type:       platformwebhook.EventTest,
			ObjectType: "webhook_endpoint",
			ObjectID:   &endpointID,
			Payload:    payload,
			OccurredAt: s.now().UTC(),
		}, endpointID)
		deliveryID = createdDeliveryID
		return emitErr
	})
	if err != nil {
		return Delivery{}, apperrors.NewInternal("Unable to create webhook test delivery", err)
	}
	delivery, err := s.repository.GetDelivery(ctx, deliveryID, tenantContext.Scope.TeamID)
	if err != nil {
		return Delivery{}, apperrors.NewInternal("Unable to get webhook test delivery", err)
	}
	return delivery, nil
}

func (s *Service) ListEvents(ctx context.Context, req ListRequest) ([]Event, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksRead)
	if err != nil {
		return nil, err
	}
	normalizeListRequest(&req)
	events, err := s.repository.ListEvents(ctx, tenantContext.Scope.TeamID, req.Limit, req.Offset)
	if err != nil {
		return nil, apperrors.NewInternal("Unable to list webhook events", err)
	}
	return events, nil
}

func (s *Service) GetEvent(ctx context.Context, value string) (Event, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksRead)
	if err != nil {
		return Event{}, err
	}
	id, err := parseID(value, "Webhook event")
	if err != nil {
		return Event{}, err
	}
	event, err := s.repository.GetEvent(ctx, id, tenantContext.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, apperrors.NewNotFound("Webhook event not found")
	}
	if err != nil {
		return Event{}, apperrors.NewInternal("Unable to get webhook event", err)
	}
	return event, nil
}

func (s *Service) GetDelivery(ctx context.Context, value string) (Delivery, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksRead)
	if err != nil {
		return Delivery{}, err
	}
	id, err := parseID(value, "Webhook delivery")
	if err != nil {
		return Delivery{}, err
	}
	delivery, err := s.repository.GetDelivery(ctx, id, tenantContext.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, apperrors.NewNotFound("Webhook delivery not found")
	}
	if err != nil {
		return Delivery{}, apperrors.NewInternal("Unable to get webhook delivery", err)
	}
	return delivery, nil
}

func (s *Service) RetryDelivery(ctx context.Context, value string) (Delivery, error) {
	tenantContext, err := requireTenant(ctx, tenant.PermissionWebhooksWrite)
	if err != nil {
		return Delivery{}, err
	}
	id, err := parseID(value, "Webhook delivery")
	if err != nil {
		return Delivery{}, err
	}
	delivery, err := s.repository.RetryDelivery(ctx, id, tenantContext.Scope.TeamID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Delivery{}, apperrors.NewNotFound("Failed webhook delivery not found")
	}
	if err != nil {
		return Delivery{}, apperrors.NewInternal("Unable to retry webhook delivery", err)
	}
	audit.Record(ctx, tenantContext, audit.Event{Action: "webhook_delivery.retried", ResourceType: "webhook_delivery", ResourceID: delivery.ID})
	return delivery, nil
}

func requireTenant(ctx context.Context, permission tenant.Permission) (tenant.AccessContext, error) {
	tenantContext, decision := tenant.ResolveAccess(ctx, permission)
	if !decision.Allowed {
		return tenant.AccessContext{}, apperrors.NewForbidden(decision.Reason)
	}
	return tenantContext, nil
}

func parseID(value, resource string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return uuid.Nil, apperrors.NewBadRequest(resource + " id must be a valid UUID")
	}
	return id, nil
}

func normalizeListRequest(req *ListRequest) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
}
