package broadcast

import (
	"github.com/labstack/echo/v5"

	module "github.com/coffeyvidzro/dugble/server/internal/modules/broadcast"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

func (h *Handler) Duplicate(c *echo.Context) error {
	var req module.DuplicateRequest
	if err := decodeJSON(c, &req, false); err != nil {
		return err
	}
	value, err := h.service.Duplicate(c.Request().Context(), c.Param("broadcast"), req)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, value)
}
