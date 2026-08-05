package auditevent

import (
	"strconv"
	"strings"

	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
	"github.com/labstack/echo/v5"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(c *echo.Context) error {
	var limit int32
	if value := strings.TrimSpace(c.QueryParam("limit")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return httputil.Error(c, apperrors.NewBadRequest("Audit event limit must be an integer"))
		}
		limit = int32(parsed)
	}
	response, err := h.service.List(c.Request().Context(), strings.TrimSpace(c.QueryParam("before")), limit)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, response)
}
