package domains

import (
	"context"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	backofficedomains "github.com/coffeyvidzro/dugble/server/internal/backoffice/domains"
	backofficehttp "github.com/coffeyvidzro/dugble/server/internal/transport/backoffice"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

type service interface {
	List(context.Context, backofficedomains.ListInput) (backofficedomains.Page, error)
	Get(context.Context, string) (backofficedomains.Domain, error)
}

type Handler struct {
	service service
}

type listPage struct {
	Domains        []backofficedomains.Domain
	Limit          int32
	Offset         int32
	PreviousOffset int32
	NextOffset     int32
	HasPrevious    bool
	HasNext        bool
}

func NewHandler(service service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	limit, err := parseInt32Query(c.QueryParam("limit"), "limit")
	if err != nil {
		return err
	}
	offset, err := parseInt32Query(c.QueryParam("offset"), "offset")
	if err != nil {
		return err
	}

	page, err := handler.service.List(c.Request().Context(), backofficedomains.ListInput{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "domains.html", backofficehttp.PageData{
		Title: "Domains",
		Data:  newListPage(page),
	})
}

func (handler *Handler) Detail(c *echo.Context) error {
	domain, err := handler.service.Get(c.Request().Context(), c.Param("domain_id"))
	if err != nil {
		return err
	}

	return backofficehttp.RenderPage(c, "domain_detail.html", backofficehttp.PageData{
		Title: domain.Name,
		Data:  domain,
	})
}

func newListPage(page backofficedomains.Page) listPage {
	previousOffset := page.Offset - page.Limit
	if previousOffset < 0 {
		previousOffset = 0
	}

	return listPage{
		Domains:        page.Domains,
		Limit:          page.Limit,
		Offset:         page.Offset,
		PreviousOffset: previousOffset,
		NextOffset:     page.Offset + page.Limit,
		HasPrevious:    page.Offset > 0,
		HasNext:        len(page.Domains) == int(page.Limit),
	}
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
