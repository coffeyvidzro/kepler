package domain

import (
	"encoding/json"
	"strconv"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *echo.Context) error {
	domains, err := h.service.List(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, domains)
}

func (h *Handler) Get(c *echo.Context) error {
	domain, err := h.service.Get(c.Request().Context(), c.Param("domain_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, domain)
}

func (h *Handler) Create(c *echo.Context) error {
	var req CreateRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	result, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	if result.Provisioning {
		c.Response().Header().Set("Retry-After", strconv.Itoa(emailInfrastructureRetryAfterSeconds))
		return httputil.Accepted(c, ProvisioningResponse{
			Status:            "provisioning",
			Message:           emailInfrastructureProvisioningMessage,
			RetryAfterSeconds: emailInfrastructureRetryAfterSeconds,
		})
	}
	return httputil.Created(c, result.Domain)
}

func (h *Handler) Verify(c *echo.Context) error {
	domain, err := h.service.Verify(c.Request().Context(), c.Param("domain_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, domain)
}

func (h *Handler) Delete(c *echo.Context) error {
	domain, err := h.service.Delete(c.Request().Context(), c.Param("domain_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, domain)
}

func decodeJSON(c *echo.Context, dst any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(dst); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}
