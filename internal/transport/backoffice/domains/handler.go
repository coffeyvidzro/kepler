package domains

import (
	"context"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	backofficedomains "github.com/coffeyvidzro/dugble/server/internal/backoffice/domains"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type service interface {
	List(context.Context, backofficedomains.ListInput) (backofficedomains.Page, error)
	Get(context.Context, string) (backofficedomains.Domain, error)
}

type Handler struct {
	service service
}

func NewHandler(service service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	limit, err := parseInt32Query(c.QueryParam("limit"), "limit")
	if err != nil {
		return httputil.Error(c, err)
	}
	offset, err := parseInt32Query(c.QueryParam("offset"), "offset")
	if err != nil {
		return httputil.Error(c, err)
	}
	page, err := handler.service.List(c.Request().Context(), backofficedomains.ListInput{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, page)
}

func (handler *Handler) Get(c *echo.Context) error {
	domain, err := handler.service.Get(c.Request().Context(), c.Param("domain_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, domain)
}

func parseInt32Query(value string, name string) (int32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, apperrors.NewBadRequest("Invalid " + name)
	}
	return int32(parsed), nil
}
