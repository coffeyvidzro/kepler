package broadcast

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/labstack/echo/v5"

	module "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(c *echo.Context) error {
	var req module.CreateRequest
	if err := decodeJSON(c, &req, false); err != nil { return err }
	value, err := h.service.Create(c.Request().Context(), req)
	if err != nil { return httputil.Error(c, err) }
	return httputil.Created(c, value)
}
func (h *Handler) List(c *echo.Context) error {
	values, err := h.service.List(c.Request().Context(), module.ListRequest{Limit: parseInt32(c.QueryParam("limit")), Offset: parseInt32(c.QueryParam("offset"))})
	if err != nil { return httputil.Error(c, err) }
	return httputil.OK(c, values)
}
func (h *Handler) Get(c *echo.Context) error {
	value, err := h.service.Get(c.Request().Context(), c.Param("broadcast"))
	if err != nil { return httputil.Error(c, err) }
	return httputil.OK(c, value)
}
func (h *Handler) Update(c *echo.Context) error {
	var req module.UpdateRequest
	if err := decodeJSON(c, &req, false); err != nil { return err }
	value, err := h.service.Update(c.Request().Context(), c.Param("broadcast"), req)
	if err != nil { return httputil.Error(c, err) }
	return httputil.OK(c, value)
}
func (h *Handler) Delete(c *echo.Context) error {
	value, err := h.service.Delete(c.Request().Context(), c.Param("broadcast"))
	if err != nil { return httputil.Error(c, err) }
	return httputil.OK(c, value)
}
func (h *Handler) Send(c *echo.Context) error {
	var req module.SendRequest
	if err := decodeJSON(c, &req, true); err != nil { return err }
	value, err := h.service.Send(c.Request().Context(), c.Param("broadcast"), req)
	if err != nil { return httputil.Error(c, err) }
	return httputil.Accepted(c, value)
}
func (h *Handler) Cancel(c *echo.Context) error {
	value, err := h.service.Cancel(c.Request().Context(), c.Param("broadcast"))
	if err != nil { return httputil.Error(c, err) }
	return httputil.OK(c, value)
}
func (h *Handler) Preview(c *echo.Context) error {
	var req module.PreviewRequest
	if err := decodeJSON(c, &req, true); err != nil { return err }
	value, err := h.service.Preview(c.Request().Context(), c.Param("broadcast"), req)
	if err != nil { return httputil.Error(c, err) }
	return httputil.OK(c, value)
}
func (h *Handler) ListRecipients(c *echo.Context) error {
	values, err := h.service.ListRecipients(c.Request().Context(), c.Param("broadcast"), module.ListRequest{Limit: parseInt32(c.QueryParam("limit")), Offset: parseInt32(c.QueryParam("offset"))})
	if err != nil { return httputil.Error(c, err) }
	return httputil.OK(c, values)
}

func decodeJSON(c *echo.Context, dst any, optional bool) error {
	decoder := json.NewDecoder(c.Request().Body)
	decoder.UseNumber()
	if err := decoder.Decode(dst); err != nil {
		if optional && errors.Is(err, io.EOF) { return nil }
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	return nil
}
func parseInt32(value string) int32 {
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil { return 0 }
	return int32(parsed)
}
