package senderid

import (
	"encoding/json"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *echo.Context) error {
	senderIDs, err := h.service.List(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, senderIDs)
}

func (h *Handler) Get(c *echo.Context) error {
	senderID, err := h.service.Get(c.Request().Context(), c.Param("sender_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, senderID)
}

func (h *Handler) Create(c *echo.Context) error {
	var req CreateRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	senderID, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, senderID)
}

func (h *Handler) Delete(c *echo.Context) error {
	senderID, err := h.service.Delete(c.Request().Context(), c.Param("sender_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, senderID)
}

func decodeJSON(c *echo.Context, dst any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(dst); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}
