package session

import (
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

func (h *Handler) List(c *echo.Context) error {
	sessions, err := h.service.List(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, sessions)
}

func (h *Handler) Get(c *echo.Context) error {
	id, err := validateSessionID(c.Param("id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	sessions, err := h.service.List(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	for _, session := range sessions {
		if session.ID == id {
			return httputil.OK(c, session)
		}
	}
	return httputil.Error(c, apperrors.NewNotFound("Session not found"))
}

func (h *Handler) Revoke(c *echo.Context) error {
	if err := h.service.Revoke(c.Request().Context(), c.Param("id")); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"revoked": true})
}

func (h *Handler) RevokeOthers(c *echo.Context) error {
	if err := h.service.RevokeOthers(c.Request().Context()); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"revoked": true})
}

func (h *Handler) RevokeAll(c *echo.Context) error {
	if err := h.service.RevokeAll(c.Request().Context()); err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, map[string]bool{"revoked": true})
}
