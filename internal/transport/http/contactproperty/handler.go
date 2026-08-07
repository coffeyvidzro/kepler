package contactproperty

import (
	"net/http"

	"github.com/labstack/echo/v5"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(c *echo.Context) error {
	if h == nil || h.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Contact property service is not configured", nil))
	}
	var req CreateRequest
	if err := httputil.DecodeJSON(c, &req, httputil.DefaultMaxRequestBodyBytes); err != nil {
		return httputil.Error(c, err)
	}
	value, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, value)
}

func (h *Handler) List(c *echo.Context) error {
	if h == nil || h.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Contact property service is not configured", nil))
	}
	values, err := h.service.List(c.Request().Context(), ListRequest{
		Limit:  httputil.QueryInt32(c, "limit"),
		After:  c.QueryParam("after"),
		Before: c.QueryParam("before"),
	})
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, values)
}

func (h *Handler) Get(c *echo.Context) error {
	if h == nil || h.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Contact property service is not configured", nil))
	}
	value, err := h.service.Get(c.Request().Context(), c.Param("property_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, value)
}

func (h *Handler) Update(c *echo.Context) error {
	if h == nil || h.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Contact property service is not configured", nil))
	}
	var req UpdateRequest
	if err := httputil.DecodeJSON(c, &req, httputil.DefaultMaxRequestBodyBytes); err != nil {
		return httputil.Error(c, err)
	}
	value, err := h.service.Update(c.Request().Context(), c.Param("property_id"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, value)
}

func (h *Handler) Delete(c *echo.Context) error {
	if h == nil || h.service == nil {
		return httputil.Error(c, apperrors.NewServiceUnavailable("Contact property service is not configured", nil))
	}
	value, err := h.service.Delete(c.Request().Context(), c.Param("property_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, value)
}
