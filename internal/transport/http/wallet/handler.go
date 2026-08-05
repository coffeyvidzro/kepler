package wallet

import (
	"strconv"
	"strings"

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

func (h *Handler) Get(c *echo.Context) error {
	wallet, err := h.service.Get(c.Request().Context())
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, wallet)
}

func (h *Handler) ListLedger(c *echo.Context) error {
	limit, err := parseInt32Query(c, "limit")
	if err != nil {
		return httputil.Error(c, err)
	}
	offset, err := parseInt32Query(c, "offset")
	if err != nil {
		return httputil.Error(c, err)
	}
	page, err := h.service.ListLedger(c.Request().Context(), limit, offset)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, page)
}

func parseInt32Query(c *echo.Context, name string) (int32, error) {
	value := strings.TrimSpace(c.QueryParam(name))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, apperrors.NewBadRequest("Wallet " + name + " must be an integer")
	}
	return int32(parsed), nil
}
