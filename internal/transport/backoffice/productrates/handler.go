package productrates

import (
	"encoding/json"
	"strconv"

	"github.com/labstack/echo/v5"

	backofficeproductrates "github.com/coffeyvidzro/dugble/server/internal/backoffice/productrates"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *backofficeproductrates.Service }

func NewHandler(service *backofficeproductrates.Service) *Handler {
	return &Handler{service: service}
}

func (handler *Handler) List(c *echo.Context) error {
	limit, err := parseInt32(c.QueryParam("limit"))
	if err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid limit"))
	}
	offset, err := parseInt32(c.QueryParam("offset"))
	if err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid offset"))
	}
	page, err := handler.service.List(c.Request().Context(), backofficeproductrates.ListInput{Limit: limit, Offset: offset})
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, page)
}

func (handler *Handler) Get(c *echo.Context) error {
	item, err := handler.service.Get(c.Request().Context(), c.Param("rate_id"))
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, item)
}

func (handler *Handler) Create(c *echo.Context) error {
	var input backofficeproductrates.CreateInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	item, err := handler.service.Create(c.Request().Context(), input)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.Created(c, item)
}

func (handler *Handler) Close(c *echo.Context) error {
	var input backofficeproductrates.CloseInput
	if err := json.NewDecoder(c.Request().Body).Decode(&input); err != nil {
		return httputil.Error(c, apperrors.NewBadRequest("Invalid JSON request body"))
	}
	item, err := handler.service.Close(c.Request().Context(), c.Param("rate_id"), input)
	if err != nil {
		return httputil.Error(c, err)
	}
	return httputil.OK(c, item)
}

func parseInt32(value string) (int32, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	return int32(parsed), err
}
