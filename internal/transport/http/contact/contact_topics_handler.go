package contact

import (
	"net/http"

	"github.com/labstack/echo/v5"

	contactmodule "github.com/coffeyvidzro/dugble/server/internal/modules/contact"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

func (h *Handler) ListTopics(c *echo.Context) error {
	response, err := h.service.ListTopics(c.Request().Context(), c.Param("contact_id"), contactmodule.ListContactTopicsRequest{
		Limit:  httputil.QueryInt32(c, "limit"),
		After:  c.QueryParam("after"),
		Before: c.QueryParam("before"),
	})
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, response)
}

func (h *Handler) UpdateTopics(c *echo.Context) error {
	var request contactmodule.UpdateContactTopicsRequest
	if err := httputil.DecodeJSON(c, &request, httputil.DefaultMaxRequestBodyBytes); err != nil {
		return httputil.Error(c, err)
	}
	response, err := h.service.UpdateTopics(c.Request().Context(), c.Param("contact_id"), request)
	if err != nil {
		return httputil.Error(c, err)
	}
	return c.JSON(http.StatusOK, response)
}
