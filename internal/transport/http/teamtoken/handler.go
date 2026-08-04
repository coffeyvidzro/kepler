package teamtoken

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

func (h *Handler) List(c *echo.Context) error {
	tokens, err := h.service.List(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, tokens)
}
func (h *Handler) Create(c *echo.Context) error {
	var req CreateRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	token, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, token)
}
func (h *Handler) Update(c *echo.Context) error {
	var req UpdateRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	token, err := h.service.Update(c.Request().Context(), c.Param("token_id"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, token)
}
func (h *Handler) Revoke(c *echo.Context) error {
	token, err := h.service.Revoke(c.Request().Context(), c.Param("token_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, token)
}

func decodeJSON(c *echo.Context, dst any) error {
	if err := json.NewDecoder(c.Request().Body).Decode(dst); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}
