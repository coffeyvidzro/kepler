package mfa

import (
	"encoding/json"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Enroll(c *echo.Context) error {
	response, err := h.service.Enroll(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, response)
}

func (h *Handler) Confirm(c *echo.Context) error {
	return h.withCode(c, func(ctxCode string) (any, error) { return h.service.Confirm(c.Request().Context(), ctxCode) })
}
func (h *Handler) Verify(c *echo.Context) error {
	return h.withCode(c, func(code string) (any, error) {
		return map[string]bool{"verified": true}, h.service.Verify(c.Request().Context(), code)
	})
}
func (h *Handler) Recover(c *echo.Context) error {
	return h.withCode(c, func(code string) (any, error) {
		return map[string]bool{"verified": true}, h.service.Recover(c.Request().Context(), code)
	})
}

func (h *Handler) Disable(c *echo.Context) error {
	if err := h.service.Disable(c.Request().Context()); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"enabled": false})
}

func (h *Handler) Status(c *echo.Context) error {
	response, err := h.service.Status(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, response)
}

func (h *Handler) withCode(c *echo.Context, action func(string) (any, error)) error {
	var request CodeRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&request); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	response, err := action(request.Code)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, response)
}
