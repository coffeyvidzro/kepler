package sms

import (
	"encoding/json"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *echo.Context) error {
	messages, err := h.service.List(c.Request().Context(), ListRequest{Limit: parseInt32(c.QueryParam("limit")), Offset: parseInt32(c.QueryParam("offset"))})
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, Responses(messages))
}

func (h *Handler) Get(c *echo.Context) error {
	message, err := h.service.Get(c.Request().Context(), c.Param("message_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, message.Response())
}

func (h *Handler) Send(c *echo.Context) error {
	if _, err := idempotency.ValidateKey(c.Request().Header.Get(idempotency.Header)); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Idempotency-Key is required and must be at most 256 characters"))
	}
	var req SendRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	message, err := h.service.Send(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	c.Response().Header().Set("Location", "/sms/"+message.ID)
	return httputil.Accepted(c, message.SendResponse())
}

func (h *Handler) BatchSend(c *echo.Context) error {
	var req BatchSendRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	response, err := h.service.BatchSend(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Accepted(c, SendResponses(response))
}

func (h *Handler) Cancel(c *echo.Context) error {
	response, err := h.service.Cancel(c.Request().Context(), c.Param("message_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, response)
}

func (h *Handler) Update(c *echo.Context) error {
	var req UpdateRequest
	if json.NewDecoder(c.Request().Body).Decode(&req) != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	response, err := h.service.Update(c.Request().Context(), c.Param("message_id"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, response)
}

func (h *Handler) SyncStatus(c *echo.Context) error {
	message, err := h.service.SyncStatus(c.Request().Context(), c.Param("message_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, message.Response())
}

func parseInt32(value string) int32 {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0
	}
	return int32(parsed)
}
