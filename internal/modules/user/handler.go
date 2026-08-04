package user

import (
	"encoding/json"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetMe(c *echo.Context) error {
	user, err := h.service.GetMe(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, user)
}

func (h *Handler) UpdateMe(c *echo.Context) error {
	var req UpdateProfileRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	updated, err := h.service.UpdateProfile(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, updated)
}

func (h *Handler) DeleteMe(c *echo.Context) error {
	if err := h.service.DeleteMe(c.Request().Context()); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"deleted": true})
}

func (h *Handler) UpdatePassword(c *echo.Context) error {
	var req UpdatePasswordRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	updated, err := h.service.UpdatePassword(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, updated)
}

func (h *Handler) UpdateEmail(c *echo.Context) error {
	var req UpdateEmailRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	updated, err := h.service.UpdateEmail(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, updated)
}
