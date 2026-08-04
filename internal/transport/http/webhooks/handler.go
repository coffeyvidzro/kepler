package webhooks

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) CreateEndpoint(c *echo.Context) error {
	var req CreateEndpointRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	endpoint, err := h.service.CreateEndpoint(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, endpoint)
}

func (h *Handler) ListEndpoints(c *echo.Context) error {
	endpoints, err := h.service.ListEndpoints(c.Request().Context(), listRequest(c))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, endpoints)
}

func (h *Handler) GetEndpoint(c *echo.Context) error {
	endpoint, err := h.service.GetEndpoint(c.Request().Context(), c.Param("endpoint_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, endpoint)
}

func (h *Handler) UpdateEndpoint(c *echo.Context) error {
	var req UpdateEndpointRequest
	if err := decodeJSON(c, &req); err != nil {
		return err
	}
	endpoint, err := h.service.UpdateEndpoint(c.Request().Context(), c.Param("endpoint_id"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, endpoint)
}

func (h *Handler) DeleteEndpoint(c *echo.Context) error {
	if err := h.service.DeleteEndpoint(c.Request().Context(), c.Param("endpoint_id")); err != nil {
		return httputil.Error(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) TestEndpoint(c *echo.Context) error {
	delivery, err := h.service.TestEndpoint(c.Request().Context(), c.Param("endpoint_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, delivery)
}

func (h *Handler) RotateSecret(c *echo.Context) error {
	endpoint, err := h.service.RotateSecret(c.Request().Context(), c.Param("endpoint_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, endpoint)
}

func (h *Handler) ListEvents(c *echo.Context) error {
	events, err := h.service.ListEvents(c.Request().Context(), listRequest(c))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, events)
}

func (h *Handler) GetEvent(c *echo.Context) error {
	event, err := h.service.GetEvent(c.Request().Context(), c.Param("event_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, event)
}

func (h *Handler) GetDelivery(c *echo.Context) error {
	delivery, err := h.service.GetDelivery(c.Request().Context(), c.Param("delivery_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, delivery)
}

func (h *Handler) RetryDelivery(c *echo.Context) error {
	delivery, err := h.service.RetryDelivery(c.Request().Context(), c.Param("delivery_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, delivery)
}

func decodeJSON(c *echo.Context, destination any) error {
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}

func listRequest(c *echo.Context) ListRequest {
	limit, _ := strconv.ParseInt(c.QueryParam("limit"), 10, 32)
	offset, _ := strconv.ParseInt(c.QueryParam("offset"), 10, 32)
	return ListRequest{Limit: int32(limit), Offset: int32(offset)}
}
